import os
import io
import threading
from fastapi import FastAPI, UploadFile, File, HTTPException
from pydantic import BaseModel
from PIL import Image
from sentence_transformers import SentenceTransformer

MODEL_NAME = os.environ.get("EMBED_MODEL", "clip-ViT-B-32")

print(f"loading model: {MODEL_NAME}")
model = SentenceTransformer(MODEL_NAME)
DIM = model.encode("test").shape[0]
print(f"model loaded: dim={DIM}")

app = FastAPI()

# state
_model = model
_model_name = MODEL_NAME
_dim = DIM
_loading = False
_lock = threading.Lock()

class TextEmbedRequest(BaseModel):
    texts: list[str]

class ModelSwapRequest(BaseModel):
    model: str

@app.get("/health")
def health():
    if _loading:
        return {"status": "loading", "model": _model_name, "dim": 0}
    return {"status": "ok", "model": _model_name, "dim": _dim}

@app.post("/embed/text")
def embed_text(req: TextEmbedRequest):
    if _loading:
        raise HTTPException(status_code=503, detail="model is loading")
    with _lock:
        vectors = _model.encode(req.texts, normalize_embeddings=True)
    return {"embeddings": vectors.tolist()}

@app.post("/embed/image")
async def embed_image(file: UploadFile = File(...)):
    if _loading:
        raise HTTPException(status_code=503, detail="model is loading")
    contents = await file.read()
    image = Image.open(io.BytesIO(contents)).convert("RGB")
    with _lock:
        vector = _model.encode(image, normalize_embeddings=True)
    return {"embedding": vector.tolist()}

@app.post("/model")
def swap_model(req: ModelSwapRequest):
    global _model, _model_name, _dim, _loading
    _loading = True
    try:
        print(f"loading model: {req.model}")
        with _lock:
            new_model = SentenceTransformer(req.model)
            new_dim = new_model.encode("test").shape[0]
            _model = new_model
            _model_name = req.model
            _dim = new_dim
        print(f"model loaded: {req.model} dim={new_dim}")
        return {"status": "ok", "model": _model_name, "dim": _dim}
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"failed to load model: {e}")
    finally:
        _loading = False