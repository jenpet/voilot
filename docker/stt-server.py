"""faster-whisper STT HTTP server for voilot.

Exposes a simple HTTP API for speech-to-text transcription.

Endpoints:
  POST /transcribe  - Transcribe audio to text
  GET  /health      - Health check
"""

import io
import os
import tempfile

from flask import Flask, jsonify, request
from faster_whisper import WhisperModel

app = Flask(__name__)

MODEL_SIZE = os.environ.get("WHISPER_MODEL", "base")
DEVICE = os.environ.get("WHISPER_DEVICE", "cpu")
COMPUTE_TYPE = os.environ.get("WHISPER_COMPUTE_TYPE", "int8")
PORT = int(os.environ.get("STT_PORT", "5003"))

# Initialize model at startup
print(f"Loading Whisper model: {MODEL_SIZE} (device={DEVICE}, compute_type={COMPUTE_TYPE})")
model = WhisperModel(MODEL_SIZE, device=DEVICE, compute_type=COMPUTE_TYPE)
print("Whisper model loaded successfully")


# Map of audio MIME types / filename extensions to temp-file suffixes.
# The suffix matters because ffmpeg uses it to identify the container format.
_MIME_TO_EXT = {
    "audio/webm": ".webm",
    "audio/ogg": ".ogg",
    "audio/wav": ".wav",
    "audio/x-wav": ".wav",
    "audio/wave": ".wav",
    "audio/mp3": ".mp3",
    "audio/mpeg": ".mp3",
    "audio/mp4": ".m4a",
    "audio/x-m4a": ".m4a",
    "audio/flac": ".flac",
}


def _suffix_for_audio(filename, content_type) -> str:
    """Determine the correct temp-file suffix for an uploaded audio file."""
    # Try filename extension first (most reliable)
    if filename:
        _, ext = os.path.splitext(filename)
        if ext:
            return ext.lower()

    # Fall back to content type
    if content_type:
        # Strip parameters like "audio/webm;codecs=opus" -> "audio/webm"
        base_type = content_type.split(";")[0].strip().lower()
        if base_type in _MIME_TO_EXT:
            return _MIME_TO_EXT[base_type]

    # Default to .wav as a last resort
    return ".wav"


@app.route("/health", methods=["GET"])
def health():
    return jsonify({
        "status": "ok",
        "model": MODEL_SIZE,
        "device": DEVICE,
        "compute_type": COMPUTE_TYPE,
    })


@app.route("/transcribe", methods=["POST"])
def transcribe():
    """Transcribe audio to text.

    Accepts:
      - multipart/form-data with 'audio' file field
      - Query params:
        - language: optional language code (e.g., "en", "de")
        - beam_size: optional beam size (default 5)

    Returns JSON:
      {
        "text": "transcribed text",
        "segments": [...],
        "language": "en",
        "language_probability": 0.98,
        "duration": 3.5
      }
    """
    if "audio" not in request.files:
        return jsonify({"error": "Missing 'audio' file in form data"}), 400

    audio_file = request.files["audio"]
    language = request.args.get("language")
    beam_size = int(request.args.get("beam_size", "5"))

    # Determine file extension from the uploaded filename or content type
    # so ffmpeg can correctly identify the audio format.
    suffix = _suffix_for_audio(audio_file.filename, audio_file.content_type)

    tmp_path = None
    try:
        # Save uploaded audio to a temp file (faster-whisper needs a file path)
        with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
            audio_file.save(tmp)
            tmp_path = tmp.name

        # Transcribe
        segments, info = model.transcribe(
            tmp_path,
            language=language,
            beam_size=beam_size,
            vad_filter=True,
        )

        # Collect segments
        result_segments = []
        full_text = []
        for segment in segments:
            result_segments.append({
                "start": segment.start,
                "end": segment.end,
                "text": segment.text.strip(),
            })
            full_text.append(segment.text.strip())

        return jsonify({
            "text": " ".join(full_text),
            "segments": result_segments,
            "language": info.language,
            "language_probability": info.language_probability,
            "duration": info.duration,
        })

    except Exception as e:
        return jsonify({"error": str(e)}), 500

    finally:
        if tmp_path:
            try:
                os.unlink(tmp_path)
            except OSError:
                pass


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=PORT)
