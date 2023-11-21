from fastapi import FastAPI, Request
import torch
import commons
import utils
import uvicorn
import json
from models import SynthesizerTrn
from text.symbols import symbols
from text import cleaned_text_to_sequence, get_bert
from text.cleaner import clean_text
import os
import sys
import signal
from io import BytesIO
from av import open as avopen
from scipy.io import wavfile
import base64
from numba.core.errors import NumbaWarning
import warnings
warnings.simplefilter("ignore", category=NumbaWarning)
import logging
logging.getLogger("numba").setLevel(logging.WARNING)

current_dir = os.path.dirname(os.path.abspath(__file__))
app = FastAPI()

# Load Generator
hps = utils.get_hparams_from_file("./configs/config.json")
device = "cuda:0" if torch.cuda.is_available() else "cpu"
net_g = SynthesizerTrn(
    len(symbols),
    hps.data.filter_length // 2 + 1,
    hps.train.segment_size // hps.data.hop_length,
    n_speakers=hps.data.n_speakers,
    **hps.model,
).to(device)
_ = net_g.eval()
_ = utils.load_checkpoint("logs/as/G_57000.pth", net_g, None, skip_optimizer=True)

def get_text(text, language_str, hps):
  norm_text, phone, tone, word2ph = clean_text(text, language_str)
  phone, tone, language = cleaned_text_to_sequence(phone, tone, language_str)

  if hps.data.add_blank:
    phone = commons.intersperse(phone, 0)
    tone = commons.intersperse(tone, 0)
    language = commons.intersperse(language, 0)
    for i in range(len(word2ph)):
      word2ph[i] = word2ph[i] * 2
    word2ph[0] += 1
  bert = get_bert(norm_text, word2ph, language_str, device)
  del word2ph
  assert bert.shape[-1] == len(phone), phone

  if language_str == "ZH":
    bert = bert
    ja_bert = torch.zeros(768, len(phone))
  elif language_str == "JA":
    ja_bert = bert
    bert = torch.zeros(1024, len(phone))
  else:
    bert = torch.zeros(1024, len(phone))
    ja_bert = torch.zeros(768, len(phone))
  assert bert.shape[-1] == len(
    phone
  ), f"Bert seq len {bert.shape[-1]} != {len(phone)}"
  phone = torch.LongTensor(phone)
  tone = torch.LongTensor(tone)
  language = torch.LongTensor(language)
  return bert, ja_bert, phone, tone, language

def infer(text, sdp_ratio, noise_scale, noise_scale_w, length_scale, sid, language):
  bert, ja_bert, phones, tones, lang_ids = get_text(text, language, hps)
  with torch.no_grad():
    x_tst = phones.to(device).unsqueeze(0)
    tones = tones.to(device).unsqueeze(0)
    lang_ids = lang_ids.to(device).unsqueeze(0)
    bert = bert.to(device).unsqueeze(0)
    ja_bert = ja_bert.to(device).unsqueeze(0)
    x_tst_lengths = torch.LongTensor([phones.size(0)]).to(device)
    speakers = torch.LongTensor([hps.data.spk2id[sid]]).to(device)
    audio = (
      net_g.infer(
        x_tst,
        x_tst_lengths,
        speakers,
        tones,
        lang_ids,
        bert,
        ja_bert,
        sdp_ratio=sdp_ratio,
        noise_scale=noise_scale,
        noise_scale_w=noise_scale_w,
        length_scale=length_scale,
      )[0][0, 0]
      .data.cpu()
      .float()
      .numpy()
    )
    return audio

def replace_punctuation(text, i=2):
  punctuation = "，。？！"
  for char in punctuation:
    text = text.replace(char, char * i)
  return text


def wav2(i, o, format):
  inp = avopen(i, "rb")
  out = avopen(o, "wb", format=format)
  if format == "ogg":
    format = "libvorbis"

  ostream = out.add_stream(format)

  for frame in inp.decode(audio=0):
    for p in ostream.encode(frame):
      out.mux(p)

  for p in ostream.encode(None):
    out.mux(p)

  out.close()
  inp.close()

def restart():
  python = sys.executable
  os.execl(python, python, * sys.argv)

@app.post("/")
async def tts_endpoint(request: Request):
  global net_g, hps, speakers
  json_post_raw = await request.json()
  command = json_post_raw.get("command")
  text = json_post_raw.get("text").replace("/n", "")
  speaker = json_post_raw.get("speaker", "suijiSUI")
  language = json_post_raw.get("language", "ZH")
  sdp_ratio = json_post_raw.get("sdp_ratio", 0.2)
  noise_scale = json_post_raw.get("noise_scale", 0.5)
  noise_scale_w = json_post_raw.get("noise_scale_w", 0.6)
  length_scale = json_post_raw.get("length_scale", 1.2)

  try:
    if command == "/unload":
      restart()
    elif command == "/exit":
      os.kill(os.getpid(), signal.SIGTERM)
    if text == "":
      return {"code": 400, "error": "Empty text"}
    if speaker == "":
      return {"code": 400, "error": "No speaker"}
    if language not in ("JP", "ZH"):
      return "Invalid language"
    if length_scale >= 2:
      return {"code": 400, "error": "Too big length_scale"}
    if len(text) >= 250:
      return {"code": 400, "error": "Too long text(len(text)>=250)"}
  except:
    return {"code": 400, "error": "Invalid parameter"}

  with torch.no_grad():
    audio = infer(text, sdp_ratio, noise_scale, noise_scale_w, length_scale, speaker, language)

  wavBytes = None
  with BytesIO() as wav:
    wavfile.write(wav, hps.data.sampling_rate, audio)
    wavBytes = wav.getvalue()
    torch.cuda.empty_cache()

  return {"code": 0, "output": base64.b64encode(wavBytes).decode("utf-8"), "error": ""}

if __name__ == "__main__":
  uvicorn.run(app, host="0.0.0.0", port=9876, workers=1)
