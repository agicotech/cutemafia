from __future__ import annotations

import json
import mimetypes
from datetime import datetime, timezone
from pathlib import Path
from typing import Literal, Optional
from uuid import uuid4

from fastapi import Depends, FastAPI, Header, HTTPException, UploadFile
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, Field

ROOT = Path(__file__).parent
DATA = ROOT / "data"
UPLOADS = ROOT / "uploads"
KITTENS_FILE = DATA / "kittens.json"
INQUIRIES_FILE = DATA / "inquiries.json"
MAX_UPLOAD_BYTES = 100 * 1024 * 1024
ADMIN_PASSWORD = "cutemafia-admin-2026"

DATA.mkdir(exist_ok=True)
UPLOADS.mkdir(exist_ok=True)
INQUIRIES_FILE.touch(exist_ok=True)
if not INQUIRIES_FILE.read_text().strip():
    INQUIRIES_FILE.write_text("[]\n")


class Price(BaseModel):
    label: str = Field(min_length=1, max_length=80)
    value: str = Field(min_length=1, max_length=80)


class MediaAsset(BaseModel):
    url: str
    mediaType: Literal["image", "video"]
    name: Optional[str] = None


class Kitten(BaseModel):
    id: str = ""
    name: str = Field(min_length=1, max_length=80)
    birthDate: str
    color: str = Field(min_length=1, max_length=120)
    breedClass: str = Field(min_length=1, max_length=40)
    generation: str = Field(min_length=1, max_length=80)
    status: str = Field(min_length=1, max_length=80)
    description: str = Field(min_length=1, max_length=3000)
    mainPhoto: str
    featuredVideo: Optional[str] = None
    media: list[MediaAsset] = Field(default_factory=list)
    prices: list[Price] = Field(default_factory=list)


class Inquiry(BaseModel):
    name: str = Field(min_length=2, max_length=120)
    contact: str = Field(min_length=3, max_length=200)
    message: str = Field(min_length=3, max_length=3000)
    kittenId: Optional[str] = None


def read_json(path: Path) -> list[dict]:
    return json.loads(path.read_text())


def write_json(path: Path, value: list[dict]) -> None:
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def require_admin(x_admin_password: Optional[str] = Header(None)) -> None:
    if x_admin_password != ADMIN_PASSWORD:
        raise HTTPException(401, "Неверный пароль")


app = FastAPI(title="Cute mafia API", version="1.0.0")
app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://127.0.0.1:5173", "http://localhost:5173", "http://127.0.0.1:4173", "http://localhost:4173"],
    allow_methods=["*"],
    allow_headers=["*"],
)
app.mount("/uploads", StaticFiles(directory=UPLOADS), name="uploads")


@app.get("/api/health")
def health() -> dict:
    return {"status": "ok"}


@app.get("/api/kittens", response_model=list[Kitten])
def list_kittens() -> list[dict]:
    return read_json(KITTENS_FILE)


@app.get("/api/kittens/{kittenId}", response_model=Kitten)
def get_kitten(kittenId: str) -> dict:
    kitten = next((item for item in read_json(KITTENS_FILE) if item["id"] == kittenId), None)
    if not kitten:
        raise HTTPException(404, "Котёнок не найден")
    return kitten


@app.post("/api/kittens", response_model=Kitten, status_code=201)
def create_kitten(payload: Kitten, _: None = Depends(require_admin)) -> dict:
    kittens = read_json(KITTENS_FILE)
    kitten = payload.model_dump()
    kitten["id"] = kitten["id"] or f"kitten-{uuid4().hex[:10]}"
    if any(item["id"] == kitten["id"] for item in kittens):
        raise HTTPException(409, "Такой id уже существует")
    kittens.append(kitten)
    write_json(KITTENS_FILE, kittens)
    return kitten


@app.put("/api/kittens/{kittenId}", response_model=Kitten)
def update_kitten(kittenId: str, payload: Kitten, _: None = Depends(require_admin)) -> dict:
    kittens = read_json(KITTENS_FILE)
    index = next((i for i, item in enumerate(kittens) if item["id"] == kittenId), None)
    if index is None:
        raise HTTPException(404, "Котёнок не найден")
    kitten = payload.model_dump()
    kitten["id"] = kittenId
    kittens[index] = kitten
    write_json(KITTENS_FILE, kittens)
    return kitten


@app.delete("/api/kittens/{kittenId}", status_code=204)
def remove_kitten(kittenId: str, _: None = Depends(require_admin)) -> None:
    kittens = read_json(KITTENS_FILE)
    updated = [item for item in kittens if item["id"] != kittenId]
    if len(updated) == len(kittens):
        raise HTTPException(404, "Котёнок не найден")
    write_json(KITTENS_FILE, updated)


@app.post("/api/uploads", response_model=MediaAsset, status_code=201)
def upload_media(file: UploadFile, _: None = Depends(require_admin)) -> dict:
    content_type = file.content_type or ""
    if content_type == "application/octet-stream":
        content_type = mimetypes.guess_type(file.filename or "")[0] or content_type
    if not (content_type.startswith("image/") or content_type.startswith("video/")):
        raise HTTPException(415, "Можно загружать только изображения и видео")
    suffix = Path(file.filename or "media").suffix.lower()[:10]
    filename = f"{uuid4().hex}{suffix}"
    target = UPLOADS / filename
    size = 0
    with target.open("wb") as output:
        while chunk := file.file.read(1024 * 1024):
            size += len(chunk)
            if size > MAX_UPLOAD_BYTES:
                output.close()
                target.unlink(missing_ok=True)
                raise HTTPException(413, "Файл больше 100 МиБ")
            output.write(chunk)
    return {"url": f"/uploads/{filename}", "mediaType": content_type.split("/", 1)[0], "name": file.filename}


@app.post("/api/inquiries", status_code=201)
def create_inquiry(payload: Inquiry) -> dict:
    inquiries = read_json(INQUIRIES_FILE)
    inquiry = {
        "id": f"inquiry-{uuid4().hex[:10]}",
        **payload.model_dump(),
        "status": "new",
        "createdAt": datetime.now(timezone.utc).isoformat(),
    }
    inquiries.append(inquiry)
    write_json(INQUIRIES_FILE, inquiries)
    return inquiry
