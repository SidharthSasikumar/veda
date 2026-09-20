# Local model decision

Inspected on September 20, 2026: MacBook Pro, Apple M5 Pro, 15 CPU cores, 16 GPU cores, 24 GB unified memory; macOS 26.5.2. No device serial numbers or identifiers are retained.

The selected profile uses Qwen3.5-9B Q4_K_M with llama.cpp Metal. A small context and one slot limit extra memory use while Docker and macOS share the same unified memory. This is a resource-budget choice, not a claim that the model cannot support a larger context. The official model's maximum context is much larger; Veda deliberately limits its input and output to keep this prototype practical.

- Base model: [Qwen/Qwen3.5-9B](https://huggingface.co/Qwen/Qwen3.5-9B)
- Quantized artifact: [unsloth/Qwen3.5-9B-GGUF](https://huggingface.co/unsloth/Qwen3.5-9B-GGUF)
- Revision: `3885219b6810b007914f3a7950a8d1b469d598a5`
- File: `Qwen3.5-9B-Q4_K_M.gguf`
- Size: `5680522464` bytes (5.68 GB decimal)
- SHA-256: `03b74727a860a56338e042c4420bb3f04b2fec5734175f4cb9fa853daf52b7e8`
- Installed runtime: llama.cpp 0.4.0, build 10809, commit `5266f24da`
- Model endpoint: `http://127.0.0.1:18080/v1`
- Context: 8,192 tokens; parallel slots: 1; generation cap: 3,200 tokens
- Thinking: off; temperature: 0.2; structured JSON schemas

[llama.cpp's server documentation](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md) describes the local chat-completions interface and inference settings. The runtime is bound to loopback with its web UI disabled and CORS restricted to the same local model origin.

Port 18080 was selected because an existing application used 8080. Veda does not stop or reconfigure the other application.

Profiles recommend 4B for machines below 24 GB and 9B from 24 GB upwards. The prototype deliberately makes no automatic jump to much larger models on bigger hardware: that would need a separate quality and memory evaluation. Hardware auto-detection currently reads macOS system information; on other systems supply a profile explicitly.

The 9B download is pinned and verified. The 4B profile is configuration support only; its weights are not bundled or automatically downloaded. Any alternative model must be made available separately.
