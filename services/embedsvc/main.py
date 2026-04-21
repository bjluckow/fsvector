import os
import io
import numpy as np
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
 
@app.post("/embed/image/batch")
async def embed_image_batch(files: list[UploadFile] = File(...)):
    """Batch CLIP image embedding.
 
    Accepts multipart files, returns embeddings parallel to input.
    Failed images get null in their position; other images still succeed.
    """
    images = []
    valid = []
 
    for i, file in enumerate(files):
        try:
            contents = await file.read()
            img = Image.open(io.BytesIO(contents)).convert("RGB")
            images.append(img)
            valid.append(i)
        except Exception as e:
            print(f"  embed/image/batch: skip {i} ({file.filename}): {e}")
 
    embeddings = [None] * len(files)
 
    if images:
        vectors = model.encode(images, normalize_embeddings=True, batch_size=16)
        for j, idx in enumerate(valid):
            embeddings[idx] = vectors[j].tolist()
 
    return {"embeddings": embeddings}