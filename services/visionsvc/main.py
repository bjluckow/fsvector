import os
import io
from fastapi import FastAPI, UploadFile, File, HTTPException
from PIL import Image
import pytesseract
from transformers import BlipProcessor, BlipForConditionalGeneration
import torch

CAPTION_MODEL = os.environ.get("CAPTION_MODEL", "Salesforce/blip-image-captioning-base")
OCR_ENABLED = os.environ.get("VISION_OCR_ENABLED", "true").lower() == "true"

print(f"loading caption model: {CAPTION_MODEL}")
processor = BlipProcessor.from_pretrained(CAPTION_MODEL)
model = BlipForConditionalGeneration.from_pretrained(CAPTION_MODEL)
model.eval()
print(f"caption model loaded")

app = FastAPI()

@app.get("/health")
def health():
    return {
        "status": "ok",
        "caption_model": CAPTION_MODEL,
        "ocr": OCR_ENABLED,
    }

@app.post("/caption")
async def caption(file: UploadFile = File(...)):
    contents = await file.read()
    try:
        image = Image.open(io.BytesIO(contents)).convert("RGB")
        inputs = processor(image, return_tensors="pt")
        with torch.no_grad():
            out = model.generate(**inputs, max_new_tokens=50)
        text = processor.decode(out[0], skip_special_tokens=True)
        return {"caption": text.strip()}
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"captioning failed: {e}")

@app.post("/ocr")
async def ocr(file: UploadFile = File(...)):
    if not OCR_ENABLED:
        raise HTTPException(status_code=503, detail="OCR is disabled")
    contents = await file.read()
    try:
        image = Image.open(io.BytesIO(contents)).convert("RGB")
        text = pytesseract.image_to_string(image).strip()
        text = " ".join(text.split()).strip() # normalize whitespace
        return {"text": text}
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"OCR failed: {e}")