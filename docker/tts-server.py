"""Coqui TTS HTTP server for voilot.

Exposes a simple HTTP API for text-to-speech synthesis using XTTSv2.

Endpoints:
  POST /synthesize  - Convert text to speech audio
  GET  /voices      - List available voices/speakers
  GET  /health      - Health check
"""

import io
import os
import wave

from flask import Flask, jsonify, request, send_file
from TTS.api import TTS

app = Flask(__name__)

MODEL_NAME = os.environ.get("TTS_MODEL", "tts_models/multilingual/multi-dataset/xtts_v2")
PORT = int(os.environ.get("TTS_PORT", "5002"))

# Initialize TTS model at startup
print(f"Loading TTS model: {MODEL_NAME}")
tts = TTS(MODEL_NAME)
print("TTS model loaded successfully")


@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok", "model": MODEL_NAME})


@app.route("/voices", methods=["GET"])
def voices():
    """List available speaker/voice options."""
    speakers = []
    if hasattr(tts, "speakers") and tts.speakers:
        speakers = tts.speakers
    languages = []
    if hasattr(tts, "languages") and tts.languages:
        languages = tts.languages
    return jsonify({
        "speakers": speakers,
        "languages": languages,
    })


@app.route("/synthesize", methods=["POST"])
def synthesize():
    """Synthesize speech from text.

    Request JSON:
      {
        "text": "Hello world",
        "language": "en",       // optional, default "en"
        "speaker": "...",       // optional, model-specific
        "speaker_wav": "..."    // optional, path to reference audio for voice cloning
      }

    Returns: audio/wav
    """
    data = request.get_json()
    if not data or "text" not in data:
        return jsonify({"error": "Missing 'text' field"}), 400

    text = data["text"]
    language = data.get("language", "en")
    speaker = data.get("speaker")
    speaker_wav = data.get("speaker_wav")

    # XTTSv2 requires either speaker or speaker_wav; default to a built-in speaker
    if not speaker and not speaker_wav:
        if hasattr(tts, "speakers") and tts.speakers:
            speaker = tts.speakers[0]  # Default: "Claribel Dervla"
        else:
            return jsonify({"error": "Model requires speaker or speaker_wav but has no built-in speakers"}), 400

    try:
        # Generate audio
        wav = tts.tts(
            text=text,
            language=language,
            speaker=speaker,
            speaker_wav=speaker_wav,
        )

        # Convert to WAV bytes
        buf = io.BytesIO()
        sample_rate = tts.synthesizer.output_sample_rate if hasattr(tts, "synthesizer") else 22050
        with wave.open(buf, "wb") as wf:
            wf.setnchannels(1)
            wf.setsampwidth(2)  # 16-bit
            wf.setframerate(sample_rate)
            # Convert float32 samples to int16
            import numpy as np
            audio_int16 = (np.array(wav) * 32767).astype(np.int16)
            wf.writeframes(audio_int16.tobytes())

        buf.seek(0)
        return send_file(buf, mimetype="audio/wav", download_name="speech.wav")

    except Exception as e:
        return jsonify({"error": str(e)}), 500


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=PORT)
