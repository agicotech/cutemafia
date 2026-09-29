package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type fakeStorage struct {
	uploadedType string
}

func (f *fakeStorage) put(_ context.Context, file *os.File, name, contentType string) (mediaAsset, error) {
	f.uploadedType = contentType
	return mediaAsset{URL: "https://storage.example/media/test.webp", MediaType: "image", Name: &name}, nil
}

func (*fakeStorage) deleteOrphans(context.Context, map[string]bool, time.Time) (int, error) {
	return 0, nil
}

func TestAPIContract(t *testing.T) {
	drivers := []struct{ name, driver, dsn string }{{"sqlite", "sqlite", ":memory:"}}
	if dsn := os.Getenv("POSTGRES_TEST_DSN"); dsn != "" {
		drivers = append(drivers, struct{ name, driver, dsn string }{"postgres", "postgres", dsn})
	}
	for _, database := range drivers {
		t.Run(database.name, func(t *testing.T) {
			cfg := testConfig(database.driver, database.dsn)
			db, err := openStore(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.db.Close() })
			storage := &fakeStorage{}
			app := &application{cfg: cfg, store: db, storage: storage, httpClient: http.DefaultClient, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			server := httptest.NewServer(app.routes())
			t.Cleanup(server.Close)

			response := request(t, server.URL, http.MethodGet, "/api/health", "", "")
			assertStatus(t, response, http.StatusOK)

			payload := `{"id":"","name":"Луна","birthDate":"2026-01-02","color":"серебро","breedClass":"Pet","generation":"A","status":"Свободен","description":"Ласковая кошка","mainPhoto":"https://example/photo.webp","featuredVideo":null,"media":[],"prices":[{"label":"Питомец","value":"100 ₽"}]}`
			response = request(t, server.URL, http.MethodPost, "/api/kittens", payload, "wrong")
			assertStatus(t, response, http.StatusUnauthorized)
			response = request(t, server.URL, http.MethodPost, "/api/kittens", payload, "secret")
			assertStatus(t, response, http.StatusCreated)
			var created kitten
			decodeResponse(t, response, &created)
			if !strings.HasPrefix(created.ID, "kitten-") {
				t.Fatalf("server ID = %q", created.ID)
			}

			response = request(t, server.URL, http.MethodGet, "/api/kittens", "", "")
			assertStatus(t, response, http.StatusOK)
			var items []kitten
			decodeResponse(t, response, &items)
			if len(items) != 1 || items[0].Name != "Луна" {
				t.Fatalf("unexpected catalog: %#v", items)
			}

			inquiryPayload := `{"name":"Анна","contact":"@anna","message":"Хочу познакомиться","kittenId":"` + created.ID + `"}`
			response = request(t, server.URL, http.MethodPost, "/api/inquiries", inquiryPayload, "")
			assertStatus(t, response, http.StatusCreated)
			var inquiry inquiry
			decodeResponse(t, response, &inquiry)
			if inquiry.Status != "new" {
				t.Fatalf("status = %q", inquiry.Status)
			}
			pending, err := db.nextPendingInquiry(context.Background(), time.Now().Add(time.Second))
			if err != nil || pending.ID != inquiry.ID {
				t.Fatalf("queued inquiry = %#v, %v", pending, err)
			}
			response = request(t, server.URL, http.MethodPost, "/api/inquiries", `{"name":"Анна","contact":"@anna","message":"Текст"}`, "")
			assertStatus(t, response, http.StatusUnprocessableEntity)
			response.Body.Close()

			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, _ := writer.CreateFormFile("file", "cat.webp")
			part.Write(append([]byte("RIFF\x10\x00\x00\x00WEBP"), make([]byte, 32)...))
			writer.Close()
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/uploads", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.Header.Set("X-Admin-Password", "secret")
			response, err = http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			assertStatus(t, response, http.StatusCreated)
			if storage.uploadedType != "image/webp" {
				t.Fatalf("detected type = %q", storage.uploadedType)
			}

			response = request(t, server.URL, http.MethodDelete, "/api/kittens/"+created.ID, "", "secret")
			assertStatus(t, response, http.StatusNoContent)
		})
	}
}

func TestTelegramRetry(t *testing.T) {
	cfg := testConfig("sqlite", ":memory:")
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.ChatID = "123"
	cfg.retryDelays = []time.Duration{time.Minute}
	db, err := openStore(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.db.Close()

	item := inquiry{inquiryInput: inquiryInput{Name: "<Анна>", Contact: "@anna", Message: "Хочу & жду", KittenID: nil}, ID: "inquiry-test", Status: "new", CreatedAt: nowText()}
	if err := db.createInquiry(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	calls := 0
	var telegramText string
	telegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		telegramText, _ = payload["text"].(string)
		if calls == 1 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	defer telegram.Close()
	cfg.Telegram.APIHost = telegram.URL
	app := &application{cfg: cfg, store: db, httpClient: telegram.Client(), log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	app.deliverPending(context.Background())
	var status string
	var attempts int
	if err := db.db.QueryRow("SELECT notification_status, notification_attempts FROM inquiries WHERE id = 'inquiry-test'").Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempts != 1 {
		t.Fatalf("after failure: status=%s attempts=%d", status, attempts)
	}
	if !strings.Contains(telegramText, "&lt;Анна&gt;") || !strings.Contains(telegramText, "Хочу &amp; жду") {
		t.Fatalf("unsafe Telegram HTML: %s", telegramText)
	}
	if _, err := db.db.Exec("UPDATE inquiries SET notification_next_attempt_at = ? WHERE id = ?", databaseTime(time.Now().Add(-time.Minute)), item.ID); err != nil {
		t.Fatal(err)
	}
	app.deliverPending(context.Background())
	if err := db.db.QueryRow("SELECT notification_status, notification_attempts FROM inquiries WHERE id = 'inquiry-test'").Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "sent" || calls != 2 {
		t.Fatalf("after retry: status=%s calls=%d", status, calls)
	}
}

func TestExampleConfig(t *testing.T) {
	cfg, err := loadConfig("config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Uploads.MaxBytes != 100*1024*1024 || cfg.gcInterval != 24*time.Hour {
		t.Fatalf("unexpected defaults: max=%d gc=%s", cfg.Uploads.MaxBytes, cfg.gcInterval)
	}
	if cfg.Telegram.APIHost != "https://api.telegram.org" {
		t.Fatalf("telegram host = %q", cfg.Telegram.APIHost)
	}
}

func testConfig(driver, dsn string) config {
	var cfg config
	cfg.Server.AdminPassword = "secret"
	cfg.Database.Driver, cfg.Database.DSN = driver, dsn
	cfg.Uploads.MaxBytes = 100 * 1024 * 1024
	cfg.retryDelays = []time.Duration{time.Minute}
	return cfg
}

func request(t *testing.T, base, method, path, body, password string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if password != "" {
		req.Header.Set("X-Admin-Password", password)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func assertStatus(t *testing.T, response *http.Response, want int) {
	t.Helper()
	if response.StatusCode != want {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("status = %d, want %d: %s", response.StatusCode, want, body)
	}
}

func decodeResponse(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
