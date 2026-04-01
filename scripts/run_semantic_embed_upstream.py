#!/usr/bin/env python3
import json
import os
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import torch
import torch.nn.functional as F
from transformers import AutoModel, AutoTokenizer

MODEL_ID = os.environ.get("NEEDLEX_SEMANTIC_MODEL_ID", "intfloat/multilingual-e5-small")
LISTEN_HOST = os.environ.get("NEEDLEX_SEMANTIC_HOST", "127.0.0.1")
LISTEN_PORT = int(os.environ.get("NEEDLEX_SEMANTIC_PORT", "18180"))
MAX_LENGTH = int(os.environ.get("NEEDLEX_SEMANTIC_MAX_LENGTH", "512"))

print(f"[semantic-embed-upstream] loading tokenizer {MODEL_ID}", file=sys.stderr, flush=True)
tokenizer = AutoTokenizer.from_pretrained(MODEL_ID)
print(f"[semantic-embed-upstream] loading encoder {MODEL_ID} on cpu", file=sys.stderr, flush=True)
model = AutoModel.from_pretrained(MODEL_ID)
model.to("cpu")
model.eval()
print("[semantic-embed-upstream] model ready", file=sys.stderr, flush=True)


def to_input_list(value):
    if isinstance(value, str):
        return [value]
    if isinstance(value, list):
        return [str(item) for item in value]
    return []


def average_pool(last_hidden_states, attention_mask):
    last_hidden = last_hidden_states.masked_fill(~attention_mask[..., None].bool(), 0.0)
    return last_hidden.sum(dim=1) / attention_mask.sum(dim=1)[..., None]


def encode_texts(texts):
    prefixed = [f"query: {text}" if idx == 0 else f"passage: {text}" for idx, text in enumerate(texts)]
    batch = tokenizer(prefixed, max_length=MAX_LENGTH, padding=True, truncation=True, return_tensors="pt")
    with torch.no_grad():
        outputs = model(**batch)
    embeddings = average_pool(outputs.last_hidden_state, batch["attention_mask"])
    embeddings = F.normalize(embeddings, p=2, dim=1)
    return embeddings.cpu().tolist()


class Handler(BaseHTTPRequestHandler):
    def _send_json(self, status: int, payload: dict):
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/healthz":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"ok\n")
            return
        if self.path == "/v1/models":
            self._send_json(200, {"object": "list", "data": [{"id": MODEL_ID, "object": "model", "owned_by": "needlex-semantic-upstream"}]})
            return
        self._send_json(404, {"error": {"message": "not found"}})

    def do_POST(self):
        if self.path != "/v1/embeddings":
            self._send_json(404, {"error": {"message": "not found"}})
            return
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length)
        try:
            req = json.loads(raw)
        except Exception as exc:
            self._send_json(400, {"error": {"message": f"invalid json: {exc}"}})
            return
        inputs = to_input_list(req.get("input"))
        if not inputs:
            self._send_json(400, {"error": {"message": "input must be string or non-empty array"}})
            return
        vectors = encode_texts(inputs)
        data = []
        for idx, vector in enumerate(vectors):
            data.append({"object": "embedding", "index": idx, "embedding": [float(x) for x in vector]})
        payload = {
            "object": "list",
            "data": data,
            "model": req.get("model") or MODEL_ID,
            "usage": {"prompt_tokens": 0, "total_tokens": 0},
        }
        self._send_json(200, payload)

    def log_message(self, fmt, *args):
        sys.stderr.write("[semantic-embed-upstream] " + fmt % args + "\n")


if __name__ == "__main__":
    server = ThreadingHTTPServer((LISTEN_HOST, LISTEN_PORT), Handler)
    print(f"[semantic-embed-upstream] listening on http://{LISTEN_HOST}:{LISTEN_PORT}/v1", file=sys.stderr, flush=True)
    server.serve_forever()
