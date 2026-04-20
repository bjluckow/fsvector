import os
import io
import subprocess
import tempfile
from pathlib import Path
from email import message_from_bytes
import email as email_lib
import base64
import extract_msg
import tempfile

import magic
from fastapi import FastAPI, UploadFile, File, Form, HTTPException
from fastapi.responses import Response, StreamingResponse



app = FastAPI()

TEXT_FORMATS = {"pdf", "docx", "doc", "odt", "rtf", "html", "htm", "md", "txt"}
IMAGE_FORMATS = {"heic", "heif", "webp", "png", "gif", "bmp", "tiff", "tif", "jpg", "jpeg"}
AUDIO_FORMATS = {"mp3", "m4a", "wav", "aac", "ogg"}
VIDEO_FORMATS = {"mp4", "mov", "avi", "mkv"}

def ext(filename: str) -> str:
    return Path(filename).suffix.lstrip(".").lower()

@app.get("/health")
def health():
    backends = []
    for cmd, name in [
        (["convert", "--version"], "imagemagick"),
        (["pandoc", "--version"], "pandoc"),
        (["ffmpeg", "-version"], "ffmpeg"),
        (["pdftotext", "-v"], "pdftotext"),
    ]:
        try:
            subprocess.run(cmd, capture_output=True, check=True)
            backends.append(name)
        except Exception:
            pass
    return {"status": "ok", "backends": backends}

@app.post("/convert/text")
async def convert_text(
    file: UploadFile = File(...),
    target_format: str = Form(...),
):
    contents = await file.read()
    source_ext = ext(file.filename or "")
    if not source_ext:
        raise HTTPException(status_code=400, detail="could not determine source format")

    with tempfile.TemporaryDirectory() as tmpdir:
        input_path = os.path.join(tmpdir, f"input.{source_ext}")
        output_path = os.path.join(tmpdir, f"output.{target_format}")
        with open(input_path, "wb") as f:
            f.write(contents)
        _convert_text(input_path, output_path)
        with open(output_path, "rb") as f:
            result = f.read()

    return Response(content=result, media_type="text/plain")

@app.post("/convert/image")
async def convert_image(
    file: UploadFile = File(...),
    target_format: str = Form(...),
):
    contents = await file.read()
    source_ext = ext(file.filename or "")
    if not source_ext:
        raise HTTPException(status_code=400, detail="could not determine source format")

    with tempfile.TemporaryDirectory() as tmpdir:
        input_path = os.path.join(tmpdir, f"input.{source_ext}")
        output_path = os.path.join(tmpdir, f"output.{target_format}")
        with open(input_path, "wb") as f:
            f.write(contents)
        _convert_image(input_path, output_path)
        with open(output_path, "rb") as f:
            result = f.read()

    mime = magic.from_buffer(result, mime=True)
    return Response(content=result, media_type=mime)

@app.post("/convert/audio")
async def convert_audio(file: UploadFile = File(...)):
    contents = await file.read()
    source_ext = ext(file.filename or "")
    if not source_ext:
        raise HTTPException(status_code=400, detail="could not determine source format")

    with tempfile.TemporaryDirectory() as tmpdir:
        input_path = os.path.join(tmpdir, f"input.{source_ext}")
        output_path = os.path.join(tmpdir, "output.wav")
        with open(input_path, "wb") as f:
            f.write(contents)
        _normalize_audio(input_path, output_path)
        with open(output_path, "rb") as f:
            result = f.read()

    return Response(content=result, media_type="audio/wav")

@app.post("/convert/video/audio")
async def convert_video_audio(file: UploadFile = File(...)):
    contents = await file.read()
    source_ext = ext(file.filename or "")
    if not source_ext:
        raise HTTPException(status_code=400, detail="could not determine source format")

    with tempfile.TemporaryDirectory() as tmpdir:
        input_path = os.path.join(tmpdir, f"input.{source_ext}")
        output_path = os.path.join(tmpdir, "output.wav")
        with open(input_path, "wb") as f:
            f.write(contents)
        _normalize_audio(input_path, output_path)
        with open(output_path, "rb") as f:
            result = f.read()

    return Response(content=result, media_type="audio/wav")

@app.post("/convert/video/frames")
async def convert_video_frames(
    file: UploadFile = File(...),
    fps: float = Form(1.0),
):
    contents = await file.read()
    source_ext = ext(file.filename or "")
    if not source_ext:
        raise HTTPException(status_code=400, detail="could not determine source format")

    with tempfile.TemporaryDirectory() as tmpdir:
        input_path = os.path.join(tmpdir, f"input.{source_ext}")
        frames_dir = os.path.join(tmpdir, "frames")
        os.makedirs(frames_dir)

        with open(input_path, "wb") as f:
            f.write(contents)

        result = subprocess.run([
            "ffmpeg", "-i", input_path,
            "-vf", f"fps={fps}",
            "-pix_fmt", "yuvj420p",  # fix for iPhone HEVC non-full-range YUV
            "-frame_pts", "1",
            os.path.join(frames_dir, "frame_%06d.jpg"),
            "-y"
        ], capture_output=True)

        if result.returncode != 0:
            raise HTTPException(
                status_code=500,
                detail=f"frame extraction error: {result.stderr.decode()}"
            )

        # get video duration for timestamp calculation
        probe = subprocess.run([
            "ffprobe", "-v", "error",
            "-show_entries", "format=duration",
            "-of", "default=noprint_wrappers=1:nokey=1",
            input_path
        ], capture_output=True, text=True)
        duration = float(probe.stdout.strip() or "0")

        frame_files = sorted(Path(frames_dir).glob("frame_*.jpg"))

        # build multipart response
        boundary = "fsvector-frames"
        body = b""
        for i, frame_path in enumerate(frame_files):
            timestamp_ms = int((i / fps) * 1000)
            frame_data = frame_path.read_bytes()
            body += (
                f"--{boundary}\r\n"
                f"Content-Type: image/jpeg\r\n"
                f"X-Frame-Index: {i}\r\n"
                f"X-Timestamp-Ms: {timestamp_ms}\r\n"
                f"\r\n"
            ).encode()
            body += frame_data
            body += b"\r\n"
        body += f"--{boundary}--\r\n".encode()

    return Response(
        content=body,
        media_type=f"multipart/mixed; boundary={boundary}",
        headers={"X-Frame-Count": str(len(frame_files))}
    )

def _convert_text(input_path: str, output_path: str):
    ext_name = Path(input_path).suffix.lower()
    if ext_name == ".pdf":
        result = subprocess.run(
            ["pdftotext", input_path, output_path],
            capture_output=True,
        )
    else:
        result = subprocess.run(
            ["pandoc", input_path, "-o", output_path, "--to", "plain"],
            capture_output=True,
        )
    if result.returncode != 0:
        raise HTTPException(
            status_code=500,
            detail=f"conversion error: {result.stderr.decode()}"
        )

def _convert_image(input_path: str, output_path: str):
    result = subprocess.run(
        ["convert", input_path, output_path],
        capture_output=True,
    )
    if result.returncode != 0:
        raise HTTPException(
            status_code=500,
            detail=f"imagemagick error: {result.stderr.decode()}"
        )

def _normalize_audio(input_path: str, output_path: str):
    # first check if file has an audio stream
    probe = subprocess.run([
        "ffprobe", "-v", "error",
        "-select_streams", "a",
        "-show_entries", "stream=codec_type",
        "-of", "default=noprint_wrappers=1:nokey=1",
        input_path
    ], capture_output=True, text=True)
    
    if not probe.stdout.strip():
        raise HTTPException(
            status_code=422,
            detail="no audio stream found in file"
        )
    
    result = subprocess.run([
        "ffmpeg", "-i", input_path,
        "-ar", "16000",
        "-ac", "1",
        "-c:a", "pcm_s16le",
        "-vn",
        output_path, "-y"
    ], capture_output=True)
    if result.returncode != 0:
        raise HTTPException(
            status_code=500,
            detail=f"audio normalization error: {result.stderr.decode()}"
        )

@app.post("/convert/email")
async def convert_email(file: UploadFile = File(...)):
    contents = await file.read()
    source_ext = ext(file.filename or "")

    if source_ext == "msg":
        return _parse_msg(contents)
    return _parse_eml(contents)

def _parse_eml(data: bytes) -> dict:
    msg = email_lib.message_from_bytes(data)
    body = ""
    attachments = []

    for part in msg.walk():
        content_type = part.get_content_type()
        disposition = str(part.get("Content-Disposition", ""))
        filename = part.get_filename()

        if content_type == "text/plain" and "attachment" not in disposition:
            payload = part.get_payload(decode=True)
            if payload:
                body += payload.decode("utf-8", errors="replace")
        elif filename or "attachment" in disposition:
            payload = part.get_payload(decode=True)
            if payload:
                attachments.append({
                    "filename": filename or "attachment",
                    "mime": content_type,
                    "data": base64.b64encode(payload).decode()
                })

    return {
        "subject": msg.get("Subject", ""),
        "from":    msg.get("From", ""),
        "to":      msg.get("To", ""),
        "date":    msg.get("Date", ""),
        "body":    body,
        "attachments": attachments,
    }

def _parse_msg(data: bytes) -> dict:
    with tempfile.NamedTemporaryFile(suffix=".msg", delete=False) as tmp:
        tmp.write(data)
        tmp_path = tmp.name
    try:
        msg = extract_msg.Message(tmp_path)
        attachments = []
        for att in msg.attachments:
            if att.data:
                attachments.append({
                    "filename": att.longFilename or att.shortFilename or "attachment",
                    "mime":     att.mimetype or "application/octet-stream",
                    "data":     base64.b64encode(att.data).decode()
                })
        return {
            "subject": msg.subject or "",
            "from":    msg.sender or "",
            "to":      msg.to or "",
            "date":    str(msg.date) if msg.date else "",
            "body":    msg.body or "",
            "attachments": attachments,
        }
    finally:
        os.unlink(tmp_path)