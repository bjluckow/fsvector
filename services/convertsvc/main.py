import io
import os
import subprocess
import tempfile
from pathlib import Path

import magic
from fastapi import FastAPI, UploadFile, File, Form, HTTPException
from fastapi.responses import Response

app = FastAPI()

# formats we can convert to a common type for embedding
TEXT_FORMATS = {"pdf", "docx", "doc", "odt", "rtf", "html", "htm", "md", "txt"}
IMAGE_FORMATS = {"heic", "heif", "webp", "png", "gif", "bmp", "tiff", "tif", "jpg", "jpeg"}

def ext(filename: str) -> str:
    return Path(filename).suffix.lstrip(".").lower()

@app.get("/health")
def health():
    # verify backends are available
    backends = []
    try:
        subprocess.run(["convert", "--version"], capture_output=True, check=True)
        backends.append("imagemagick")
    except Exception:
        pass
    try:
        subprocess.run(["pandoc", "--version"], capture_output=True, check=True)
        backends.append("pandoc")
    except Exception:
        pass
    return {"status": "ok", "backends": backends}

@app.post("/convert")
async def convert(
    file: UploadFile = File(...),
    target_format: str = Form(...),
):
    contents = await file.read()
    source_ext = ext(file.filename or "")
    target_format = target_format.lower()

    if not source_ext:
        raise HTTPException(status_code=400, detail="could not determine source format")

    # write upload to a temp file
    with tempfile.TemporaryDirectory() as tmpdir:
        input_path = os.path.join(tmpdir, f"input.{source_ext}")
        output_path = os.path.join(tmpdir, f"output.{target_format}")

        with open(input_path, "wb") as f:
            f.write(contents)

        # route to the right backend
        if source_ext in IMAGE_FORMATS and target_format in IMAGE_FORMATS:
            _convert_image(input_path, output_path)
        elif source_ext in TEXT_FORMATS and target_format == "txt":
            _convert_text(input_path, output_path)
        else:
            raise HTTPException(
                status_code=422,
                detail=f"unsupported conversion: {source_ext} -> {target_format}"
            )

        with open(output_path, "rb") as f:
            result = f.read()

    mime = magic.from_buffer(result, mime=True)
    return Response(content=result, media_type=mime)

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

def _convert_text(input_path: str, output_path: str):
    result = subprocess.run(
        ["pandoc", input_path, "-o", output_path, "--to", "plain"],
        capture_output=True,
    )
    if result.returncode != 0:
        raise HTTPException(
            status_code=500,
            detail=f"pandoc error: {result.stderr.decode()}"
        )
