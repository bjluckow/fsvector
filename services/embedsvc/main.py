import os
import io
from fastapi import FastAPI, UploadFile, File
from pydantic import BaseModel
from PIL import Image
from sentence_transformers import SentenceTransformer

MODEL_NAME = os.environ.get("EMBED_MODEL", "clip-ViT-B-32")

print(f"loading model: {MODEL_NAME}")
model = SentenceTransformer(MODEL_NAME)
DIM = model.encode("test").shape[0]
print(f"model loaded: dim={DIM}")

app = FastAPI()

class TextEmbedRequest(BaseModel):
    texts: list[str]

@app.get("/health")
def health():
    return {"status": "ok", "model": MODEL_NAME, "dim": DIM}

@app.post("/embed/text")
def embed_text(req: TextEmbedRequest):
    vectors = model.encode(req.texts, normalize_embeddings=True)
    return {"embeddings": vectors.tolist()}

@app.post("/embed/image")
async def embed_image(file: UploadFile = File(...)):
    contents = await file.read()
    image = Image.open(io.BytesIO(contents)).convert("RGB")
    vector = model.encode(image, normalize_embeddings=True)
    return {"embedding": vector.tolist()}