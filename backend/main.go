package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type price struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type mediaAsset struct {
	URL       string  `json:"url"`
	MediaType string  `json:"mediaType"`
	Name      *string `json:"name,omitempty"`
}

type kitten struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	BirthDate     string       `json:"birthDate"`
	Color         string       `json:"color"`
	BreedClass    string       `json:"breedClass"`
	Generation    string       `json:"generation"`
	Status        string       `json:"status"`
	Description   string       `json:"description"`
	MainPhoto     string       `json:"mainPhoto"`
	FeaturedVideo *string      `json:"featuredVideo"`
	Media         []mediaAsset `json:"media"`
	Prices        []price      `json:"prices"`
}

type inquiryInput struct {
	Name     string  `json:"name"`
	Contact  string  `json:"contact"`
	Message  string  `json:"message"`
	KittenID *string `json:"kittenId"`
}

type inquiry struct {
	inquiryInput
	ID        string `json:"id"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

type mediaStorage interface {
	put(context.Context, *os.File, string, string) (mediaAsset, error)
	deleteOrphans(context.Context, map[string]bool, time.Time) (int, error)
}

type s3Storage struct {
	client     *s3.Client
	bucket     string
	prefix     string
	publicBase string
}

func newS3Storage(cfg config) *s3Storage {
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(cfg.Storage.Endpoint),
		Region:       cfg.Storage.Region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.Storage.AccessKey, cfg.Storage.SecretKey, ""),
		UsePathStyle: true,
	})
	return &s3Storage{client: client, bucket: cfg.Storage.Bucket, prefix: cfg.Storage.Prefix, publicBase: strings.TrimRight(cfg.Storage.PublicBaseURL, "/")}
}

func (s *s3Storage) put(ctx context.Context, file *os.File, filename, contentType string) (mediaAsset, error) {
	ext := extensionFor(contentType)
	key := s.prefix + randomID(16) + ext
	stat, err := file.Stat()
	if err != nil {
		return mediaAsset{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return mediaAsset{}, err
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          file,
		ContentLength: aws.Int64(stat.Size()),
		ContentType:   aws.String(contentType),
		CacheControl:  aws.String("public, max-age=31536000, immutable"),
		ACL:           types.ObjectCannedACLPublicRead,
	})
	if err != nil {
		return mediaAsset{}, err
	}
	name := filename
	return mediaAsset{URL: s.objectURL(key), MediaType: strings.SplitN(contentType, "/", 2)[0], Name: &name}, nil
}

func (s *s3Storage) deleteOrphans(ctx context.Context, referenced map[string]bool, cutoff time.Time) (int, error) {
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket), Prefix: aws.String(s.prefix),
	})
	deleted := 0
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return deleted, err
		}
		for _, object := range page.Contents {
			if object.Key == nil || object.LastModified == nil || object.LastModified.After(cutoff) || referenced[s.objectURL(*object.Key)] {
				continue
			}
			if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: object.Key}); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}

func (s *s3Storage) objectURL(key string) string {
	parts := strings.Split(key, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return s.publicBase + "/" + strings.Join(parts, "/")
}

type application struct {
	cfg        config
	store      *store
	storage    mediaStorage
	httpClient *http.Client
	log        *slog.Logger
}

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML config")
	flag.Parse()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Error("load config", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := openStore(ctx, cfg)
	if err != nil {
		log.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.db.Close()

	app := &application{
		cfg: cfg, store: db, storage: newS3Storage(cfg),
		httpClient: &http.Client{Timeout: 15 * time.Second}, log: log,
	}
	server := &http.Server{
		Addr: cfg.Server.Listen, Handler: app.routes(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 2 * time.Minute,
		WriteTimeout: 2 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20,
	}
	go app.runNotificationWorker(ctx)
	go app.runGCWorker(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()
	log.Info("server started", "listen", cfg.Server.Listen, "database", cfg.Database.Driver)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func (a *application) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", a.health)
	mux.HandleFunc("GET /api/kittens", a.listKittens)
	mux.HandleFunc("POST /api/kittens", a.admin(a.createKitten))
	mux.HandleFunc("GET /api/kittens/{kittenId}", a.getKitten)
	mux.HandleFunc("PUT /api/kittens/{kittenId}", a.admin(a.updateKitten))
	mux.HandleFunc("DELETE /api/kittens/{kittenId}", a.admin(a.deleteKitten))
	mux.HandleFunc("POST /api/uploads", a.admin(a.upload))
	mux.HandleFunc("POST /api/inquiries", a.createInquiry)
	return a.recoverPanic(a.logRequests(a.cors(mux)))
}

func (a *application) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.store.db.PingContext(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "База данных недоступна")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *application) listKittens(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.listKittens(r.Context())
	if err != nil {
		a.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *application) getKitten(w http.ResponseWriter, r *http.Request) {
	item, err := a.store.getKitten(r.Context(), r.PathValue("kittenId"))
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "Котёнок не найден")
		return
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *application) createKitten(w http.ResponseWriter, r *http.Request) {
	var item kitten
	if err := decodeJSON(w, r, &item, "id", "name", "birthDate", "color", "breedClass", "generation", "status", "description", "mainPhoto", "featuredVideo", "media", "prices"); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := validateKitten(item); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if item.ID == "" {
		item.ID = "kitten-" + randomID(5)
	}
	if err := a.store.createKitten(r.Context(), item); errors.Is(err, errConflict) {
		writeError(w, http.StatusConflict, "Такой id уже существует")
		return
	} else if err != nil {
		a.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (a *application) updateKitten(w http.ResponseWriter, r *http.Request) {
	var item kitten
	if err := decodeJSON(w, r, &item, "id", "name", "birthDate", "color", "breedClass", "generation", "status", "description", "mainPhoto", "featuredVideo", "media", "prices"); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := validateKitten(item); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	item.ID = r.PathValue("kittenId")
	if err := a.store.updateKitten(r.Context(), item); errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "Котёнок не найден")
		return
	} else if err != nil {
		a.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *application) deleteKitten(w http.ResponseWriter, r *http.Request) {
	if err := a.store.deleteKitten(r.Context(), r.PathValue("kittenId")); errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "Котёнок не найден")
		return
	} else if err != nil {
		a.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *application) createInquiry(w http.ResponseWriter, r *http.Request) {
	var input inquiryInput
	if err := decodeJSON(w, r, &input, "name", "contact", "message", "kittenId"); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := validateInquiry(input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	item := inquiry{inquiryInput: input, ID: "inquiry-" + randomID(5), Status: "new", CreatedAt: nowText()}
	if err := a.store.createInquiry(r.Context(), item); err != nil {
		a.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (a *application) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.Uploads.MaxBytes+(2<<20))
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Ожидается multipart/form-data")
		return
	}
	var partName, filename string
	var temp *os.File
	defer func() {
		if temp != nil {
			name := temp.Name()
			temp.Close()
			os.Remove(name)
		}
	}()
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(nextErr, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("Файл больше %d МиБ", a.cfg.Uploads.MaxBytes/(1024*1024)))
				return
			}
			writeError(w, http.StatusUnprocessableEntity, "Не удалось прочитать файл")
			return
		}
		if part.FormName() != "file" || part.FileName() == "" {
			part.Close()
			continue
		}
		partName, filename = part.FormName(), filepath.Base(part.FileName())
		temp, err = os.CreateTemp("", "cute-mafia-upload-*")
		if err == nil {
			var size int64
			size, err = io.Copy(temp, io.LimitReader(part, a.cfg.Uploads.MaxBytes+1))
			if err == nil && size > a.cfg.Uploads.MaxBytes {
				part.Close()
				writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("Файл больше %d МиБ", a.cfg.Uploads.MaxBytes/(1024*1024)))
				return
			}
		}
		part.Close()
		break
	}
	if partName == "" || temp == nil {
		writeError(w, http.StatusUnprocessableEntity, "Поле file обязательно")
		return
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		a.serverError(w, err)
		return
	}
	header := make([]byte, 512)
	n, err := temp.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		a.serverError(w, err)
		return
	}
	contentType := detectMediaType(header[:n])
	if contentType == "" {
		writeError(w, http.StatusUnsupportedMediaType, "Можно загружать только изображения и видео")
		return
	}
	asset, err := a.storage.put(r.Context(), temp, filename, contentType)
	if err != nil {
		a.serverError(w, fmt.Errorf("upload to storage: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, asset)
}

func (a *application) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-Admin-Password")
		want := a.cfg.Server.AdminPassword
		if len(got) != len(want) || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			writeError(w, http.StatusUnauthorized, "Неверный пароль")
			return
		}
		next(w, r)
	}
}

func (a *application) cors(next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(a.cfg.Server.AllowedOrigins))
	for _, origin := range a.cfg.Server.AllowedOrigins {
		allowed[origin] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Password")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			if origin != "" && !allowed[origin] {
				writeError(w, http.StatusForbidden, "Origin запрещён")
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (w *responseRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (a *application) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		a.log.Info("request", "method", r.Method, "path", r.URL.Path, "status", recorder.status, "duration_ms", time.Since(started).Milliseconds())
	})
}

func (a *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				a.log.Error("panic", "value", value)
				writeError(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *application) serverError(w http.ResponseWriter, err error) {
	a.log.Error("request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
}

func (a *application) runNotificationWorker(ctx context.Context) {
	if !a.cfg.Telegram.Enabled {
		return
	}
	ticker := time.NewTicker(a.cfg.pollInterval)
	defer ticker.Stop()
	for {
		a.deliverPending(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *application) deliverPending(ctx context.Context) {
	for ctx.Err() == nil {
		item, err := a.store.nextPendingInquiry(ctx, time.Now())
		if errors.Is(err, errNotFound) {
			return
		}
		if err != nil {
			a.log.Error("read notification queue", "error", err)
			return
		}
		err = a.sendTelegram(ctx, item.inquiry)
		if err == nil {
			err = a.store.markInquirySent(ctx, item.ID)
			if err == nil {
				a.log.Info("inquiry notification sent", "inquiry_id", item.ID)
				continue
			}
		}
		attempts := item.Attempts + 1
		delay := a.cfg.retryDelays[min(attempts-1, len(a.cfg.retryDelays)-1)]
		if dbErr := a.store.markInquiryRetry(ctx, item.ID, attempts, time.Now().Add(delay), err.Error()); dbErr != nil {
			a.log.Error("schedule notification retry", "error", dbErr, "inquiry_id", item.ID)
			return
		}
		a.log.Warn("inquiry notification failed", "error", err, "inquiry_id", item.ID, "retry_in", delay)
	}
}

func (a *application) sendTelegram(ctx context.Context, item inquiry) error {
	kittenID := "не выбран"
	if item.KittenID != nil && *item.KittenID != "" {
		kittenID = html.EscapeString(*item.KittenID)
	}
	createdAt := item.CreatedAt
	if parsed, err := time.Parse("2006-01-02T15:04:05.000000000Z", item.CreatedAt); err == nil {
		createdAt = parsed.In(time.FixedZone("МСК", 3*60*60)).Format("02.01.2006 15:04 МСК")
	}
	text := fmt.Sprintf("🐾 <b>Новая заявка Cute Mafia</b>\n\n<b>Имя:</b> %s\n<b>Контакт:</b> <code>%s</code>\n<b>Котёнок:</b> %s\n\n<b>Сообщение:</b>\n%s\n\n<i>%s · %s</i>",
		html.EscapeString(item.Name), html.EscapeString(item.Contact), kittenID,
		html.EscapeString(item.Message), html.EscapeString(createdAt), html.EscapeString(item.ID))
	payload, _ := json.Marshal(map[string]any{
		"chat_id": a.cfg.Telegram.ChatID, "text": text, "parse_mode": "HTML", "disable_web_page_preview": true,
	})
	endpoint := strings.TrimRight(a.cfg.Telegram.APIHost, "/")
	if a.cfg.Telegram.BotPublicID != "" {
		endpoint += "/bots/" + url.PathEscape(a.cfg.Telegram.BotPublicID) + "/sendMessage"
	} else {
		endpoint += "/bot" + a.cfg.Telegram.BotToken + "/sendMessage"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.cfg.Telegram.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.Telegram.APIKey)
	}
	response, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("telegram: %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (a *application) runGCWorker(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.gcInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			items, err := a.store.listKittens(ctx)
			if err != nil {
				a.log.Error("load media references", "error", err)
				continue
			}
			referenced := make(map[string]bool)
			for _, item := range items {
				referenced[item.MainPhoto] = true
				if item.FeaturedVideo != nil {
					referenced[*item.FeaturedVideo] = true
				}
				for _, asset := range item.Media {
					referenced[asset.URL] = true
				}
			}
			deleted, err := a.storage.deleteOrphans(ctx, referenced, time.Now().Add(-a.cfg.gcMinAge))
			if err != nil {
				a.log.Error("media garbage collection", "error", err)
			} else {
				a.log.Info("media garbage collection complete", "deleted", deleted)
			}
		}
	}
}

func validateKitten(item kitten) error {
	checks := []struct {
		name, value string
		min, max    int
	}{
		{"name", item.Name, 1, 80}, {"color", item.Color, 1, 120}, {"breedClass", item.BreedClass, 1, 40},
		{"generation", item.Generation, 1, 80}, {"status", item.Status, 1, 80}, {"description", item.Description, 1, 3000},
	}
	for _, check := range checks {
		if err := length(check.name, check.value, check.min, check.max); err != nil {
			return err
		}
	}
	if _, err := time.Parse("2006-01-02", item.BirthDate); err != nil {
		return errors.New("birthDate должен быть датой YYYY-MM-DD")
	}
	if item.Media == nil || item.Prices == nil {
		return errors.New("media и prices обязательны")
	}
	for _, asset := range item.Media {
		if asset.MediaType != "image" && asset.MediaType != "video" {
			return errors.New("mediaType должен быть image или video")
		}
	}
	for _, value := range item.Prices {
		if err := length("price.label", value.Label, 1, 80); err != nil {
			return err
		}
		if err := length("price.value", value.Value, 1, 80); err != nil {
			return err
		}
	}
	return nil
}

func validateInquiry(item inquiryInput) error {
	if err := length("name", item.Name, 2, 120); err != nil {
		return err
	}
	if err := length("contact", item.Contact, 3, 200); err != nil {
		return err
	}
	return length("message", item.Message, 3, 3000)
}

func length(name, value string, minimum, maximum int) error {
	count := utf8.RuneCountInString(value)
	if count < minimum || count > maximum {
		return fmt.Errorf("%s: длина должна быть от %d до %d", name, minimum, maximum)
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any, required ...string) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return errors.New("Некорректные входные данные")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return errors.New("Некорректные входные данные")
	}
	for _, name := range required {
		if _, exists := fields[name]; !exists {
			return fmt.Errorf("Поле %s обязательно", name)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("Некорректные входные данные")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

func randomID(bytesCount int) string {
	b := make([]byte, bytesCount)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func detectMediaType(data []byte) string {
	switch {
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "image/jpeg"
	case len(data) >= 8 && bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		return "image/gif"
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		if string(data[8:12]) == "avif" || string(data[8:12]) == "avis" {
			return "image/avif"
		}
		if string(data[8:12]) == "qt  " {
			return "video/quicktime"
		}
		return "video/mp4"
	case len(data) >= 4 && bytes.Equal(data[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}):
		return "video/webm"
	default:
		return ""
	}
}

func extensionFor(contentType string) string {
	return map[string]string{
		"image/webp": ".webp", "image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/avif": ".avif",
		"video/mp4": ".mp4", "video/quicktime": ".mov", "video/webm": ".webm",
	}[contentType]
}
