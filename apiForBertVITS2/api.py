from fastapi import FastAPI, Request
import torch
import argparse
import commons
import utils
import uvicorn
import json
import datetime
from models import SynthesizerTrn
from text.symbols import symbols
from text import cleaned_text_to_sequence, get_bert
from text.cleaner import clean_text
import torchaudio
import os
import sys
import signal
import asyncio

current_dir = os.path.dirname(os.path.abspath(__file__))
app = FastAPI()

# device = "cpu"
device = "cuda:0" if torch.cuda.is_available() else "cpu"
hps = None
net_g = None
speakers = []

def load_model_and_config(model_path, config_path):
  global net_g, hps, speakers
  hps = utils.get_hparams_from_file(config_path)

  net_g = SynthesizerTrn(
    len(symbols),
    hps.data.filter_length // 2 + 1,
    hps.train.segment_size // hps.data.hop_length,
    n_speakers=hps.data.n_speakers,
    **hps.model).to(device)
  _ = net_g.eval()
  _ = utils.load_checkpoint(model_path, net_g, None, skip_optimizer=True)

  speaker_ids = hps.data.spk2id
  speakers = list(speaker_ids.keys())

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
  bert = get_bert(norm_text, word2ph, language_str)

  assert bert.shape[-1] == len(phone)

  phone = torch.LongTensor(phone)
  tone = torch.LongTensor(tone)
  language = torch.LongTensor(language)

  return bert, phone, tone, language

def infer(text, sdp_ratio, noise_scale, noise_scale_w, length_scale, sid):
  bert, phones, tones, lang_ids = get_text(text, "ZH", hps)
  with torch.no_grad():
    x_tst = phones.to(device).unsqueeze(0)
    tones = tones.to(device).unsqueeze(0)
    lang_ids = lang_ids.to(device).unsqueeze(0)
    bert = bert.to(device).unsqueeze(0)
    x_tst_lengths = torch.LongTensor([phones.size(0)]).to(device)
    speakers = torch.LongTensor([hps.data.spk2id[sid]]).to(device)
    audio = net_g.infer(x_tst, x_tst_lengths, speakers, tones, lang_ids, bert, sdp_ratio=sdp_ratio,
                        noise_scale=noise_scale, noise_scale_w=noise_scale_w, length_scale=length_scale)[0][0, 0].data.cpu().float().numpy()
    return audio

def restart():
    python = sys.executable
    os.execl(python, python, * sys.argv)

@app.post("/")
async def tts_endpoint(request: Request):
  global net_g, hps, speakers
  json_post_raw = await request.json()
  command = json_post_raw.get('command')
  text = json_post_raw.get('text')
  speaker = json_post_raw.get('speaker')
  #speaker = "suijiSUI"
  sdp_ratio = json_post_raw.get('sdp_ratio', 0.2)
  noise_scale = json_post_raw.get('noise_scale', 0.6)
  noise_scale_w = json_post_raw.get('noise_scale_w', 0.8)
  length_scale = json_post_raw.get('length_scale', 1.0)

  if command == "/refresh":
    restart()
  elif command == "/exit":
    os.kill(os.getpid(), signal.SIGTERM)

  if text == "":
    return {"code": 400, "output": "", "error": "输入不可为空"}

  audio_output = infer(text, sdp_ratio, noise_scale, noise_scale_w, length_scale, speaker)
  output_file_name = "output.wav"
  torchaudio.save(output_file_name, torch.tensor(audio_output).unsqueeze(0), hps.data.sampling_rate)

  return {"code": 0, "output": (current_dir + "\\" + output_file_name), "error": ""}

if __name__ == "__main__":
  parser = argparse.ArgumentParser()
  parser.add_argument("-m", "--model", default="./logs/as/G_22000.pth", help="path of your model")
  parser.add_argument("-c", "--config", default="./configs/config.json", help="path of your config file")

  args = parser.parse_args()

  load_model_and_config(args.model, args.config)

  uvicorn.run(app, host='0.0.0.0', port=9876, workers=1)
