import os
import tempfile
from fastapi import FastAPI, UploadFile, File, HTTPException
import whisper

MODEL_NAME = os.environ.get("WHISPER_MODEL", "base.en")

print(f"loading whisper model: {MODEL_NAME}")
model = whisper.load_model(MODEL_NAME)
print(f"model loaded: {MODEL_NAME}")

app = FastAPI()

@app.get("/health")
def health():
    return {"status": "ok", "model": MODEL_NAME, "language": "en"}

@app.post("/transcribe")
async def transcribe(file: UploadFile = File(...)):
    contents = await file.read()
    ext = os.path.splitext(file.filename or "audio.mp3")[1] or ".mp3"
    with tempfile.NamedTemporaryFile(suffix=ext, delete=True) as tmp:
        tmp.write(contents)
        tmp.flush()
        try:
            result = model.transcribe(tmp.name)
        except Exception as e:
            raise HTTPException(status_code=500, detail=f"transcription failed: {e}")
    return {
        "text": result["text"].strip(),
        "language": result.get("language", "en"),
        "duration_seconds": result.get("duration", 0.0),
    }