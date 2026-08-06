# Ollama GPU Activation — Root Cause & Minimal Patch Plan

> Status: design review — no changes applied
> Date: 2026-08-06
> Trigger: ADR-0047 embedding probe is CPU-bound (~1–1.5h for 12k embeddings); Ollama reports `vram: 0.0 GB`.

---

## 1. Root Cause (tek cümle)

**`docker-compose.yml`'deki `ollama` servisinde hiçbir GPU device reservation tanımlı değil (`deploy.resources.reservations.devices` / `gpus:` yok), bu yüzden container `runc` runtime'ı ve `HostConfig.DeviceRequests: null` ile başlıyor ve Ollama GPU'yu göremeyip CPU'ya düşüyor — Docker daemon'da `nvidia` runtime kayıtlı olduğu halde.**

## 2. Kanıtlar

### 2.1 Compose — GPU talebi yok (kanıt)

`docker-compose.yml:14-21` — ollama servisinin TAM bloğu:

```yaml
  ollama:
    image: ollama/ollama:latest
    container_name: arca-ollama
    ports:
      - "11434:11434"
    volumes:
      - ollama-models:/root/.ollama
    restart: unless-stopped
```

Ne `gpus:`, ne `deploy.resources.reservations.devices`, ne `runtime: nvidia` — üç yöntemden hiçbiri yok. (Compose satırı 1-46 arasında başka hiçbir serviste de yok.)

### 2.2 Container inspect — GPU verilmemiş (kanıt)

`docker inspect arca-ollama` (çalıştırıldı, 2026-08-06):

```
HostConfig.DeviceRequests: null
HostConfig.Runtime: runc
```

Container GPU cihazı talep etmemiş ve `runc` (CPU) runtime'ıyla koşuyor. Ek: `docker exec arca-ollama nvidia-smi` → `exec: "nvidia-smi": executable file not found` (imajda binary yok — bu tek başına kanıt değil); `cat /proc/driver/nvidia/version` → `No such file or directory` (nvidia driver proc arayüzü container'a mount edilmemiş — GPU cihazlarının erişilemez olduğunun doğrudan göstergesi).

### 2.3 Docker daemon — nvidia runtime HAZIR (kanıt)

`docker info --format '{{json .Runtimes}}'` → `Runtimes` içinde:

```
"nvidia": {"path": "nvidia-container-runtime", ...}
```

NVIDIA Container Toolkit kurulu ve daemon'a kayıtlı. Yani altyapı hazır; eksik olan tek şey compose seviyesindeki talep. (`docker version` → Server 28.4.0.)

### 2.4 Çalışma kanıtı — Ollama CPU'da

`curl /api/ps` → `nomic-embed-text:latest | size: 0.4 GB | vram: 0.0 GB` — model tamamen CPU'da (RTX 4060 Laptop GPU host'ta `nvidia-smi` ile doğrulandı).

## 3. Docker Compose GPU Best Practice (resmi dokümantasyon)

### Güncel yöntem (Compose Deploy spec) — [docs.docker.com/compose/how-tos/gpu-support](https://docs.docker.com/compose/how-tos/gpu-support/)

> GPUs are referenced in a `compose.yaml` file using the `device` attribute from the Compose Deploy specification... **You must set the `capabilities` field. Otherwise, it returns an error on service deployment.**

Resmi örnek (birebir dokümandan):

```yaml
deploy:
  resources:
    reservations:
      devices:
        - driver: nvidia
          count: 1
          capabilities: [gpu]
```

Dokümantasyondaki özellikler: `capabilities` (zorunlu), `count` (int veya `all`; `device_ids` ile birbirini dışlar), `device_ids`, `driver` ('nvidia').

### Ollama'nın kendi resmi dokümantasyonu — [docs.ollama.com/docker](https://docs.ollama.com/docker.md)

- CPU-only: `docker run -d -v ollama:/root/.ollama -p 11434:11434 --name ollama ollama/ollama`
- **NVIDIA GPU**: önce NVIDIA Container Toolkit kurulumu + `sudo nvidia-ctk runtime configure --runtime=docker`, sonra:
  ```
  docker run -d --gpus=all -v ollama:/root/.ollama -p 11434:11434 --name ollama ollama/ollama
  ```
  `--gpus=all` komut satırı karşılığı, compose'ta yukarıdaki deploy.devices bloğudur.

### Eski / deprecated yöntemler

| Yöntem | Durum | Not |
|---|---|---|
| `runtime: nvidia` (compose top-level field) | Legacy (nvidia-docker2 çağı) | Hâlâ çalışır; resmi Docker docs GPU sayfasında ana yöntem olarak `deploy.devices` gösteriliyor; NVIDIA'nın kendi örneği artık `--gpus` |
| `gpus: all` (compose top-level, v2.3+ dönemi) | Eski ama çalışır | Compose v2.29+ `gpus` alanını hâlâ destekler; güncel dokümantasyon deploy spec'ine yönlendirir |
| `--runtime=nvidia` (docker run) | Legacy | nvidia-container-runtime'ı doğrudan seçme; `--gpus` modern karşılığı |
| `NVIDIA_VISIBLE_DEVICES=all` env tek başına | Yetersiz | Bu env imajda zaten gömülü (inspect kanıtı: `NVIDIA_VISIBLE_DEVICES=all` mevcut) ama device reservation olmadan hiçbir etkisi yok — kanıt: şu anki durum |

## 4. En Küçük Güvenli Değişiklik (uygulanmadı — plan)

Tek dosya, tek servis bloğu, 5 satır ekleme:

```yaml
  ollama:
    image: ollama/ollama:latest
    container_name: arca-ollama
    ports:
      - "11434:11434"
    volumes:
      - ollama-models:/root/.ollama
    deploy:                                     # ← EKLENEN
      resources:                                # ← EKLENEN
        reservations:                           # ← EKLENEN
          devices:                              # ← EKLENEN
            - driver: nvidia                    # ← EKLENEN
              count: 1                          # ← EKLENEN
              capabilities: [gpu]               # ← EKLENEN
    restart: unless-stopped
```

Gerekçe:
- `capabilities: [gpu]` **zorunlu** (resmi docs: "You must set this field. Otherwise, it returns an error").
- `count: 1` — tek GPU (RTX 4060); `device_ids` gerekmez.
- `driver: nvidia` — daemon'da kayıtlı runtime ile eşleşir (kanıt §2.3).
- Diğer servislere (firecrawl, qdrant, arca-pdf-inspector) **hiç dokunulmaz** — servis başına kapsam.

Uygulama adımları (onay sonrası):
1. `docker-compose.yml`'de yalnızca ollama bloğuna yukarıdaki 5 satır.
2. `docker compose up -d ollama` (yalnızca ollama recreate edilir; `docker compose up -d` de yeterli — diğerleri değişmediği için recreate edilmez).
3. §6 doğrulama adımları.

## 5. Risk Analizi

| Soru | Cevap | Kanıt/gerekçe |
|---|---|---|
| **Firecrawl etkilenir mi?** | **Hayır** | Ayrı servis (`firecrawl-pdf-service`, compose:2-12); değişiklik yalnızca ollama bloğuna; portları/env'i değişmiyor |
| **Qdrant etkilenir mi?** | **Hayır** | Ayrı servis (compose:23-31); dokunulmuyor; 6333/6334 portları ve qdrant-storage volume'ü aynı kalıyor |
| **Volume'ler etkilenir mi?** | **Hayır** | `ollama-models:/root/.ollama` bağlaması aynen kalıyor; named volume container'dan bağımsız yaşar; recreate volume'ü yeniden bağlar, içeriğini silmez |
| **Modeller yeniden indirilir mi?** | **Hayır** | Model blob'ları `/root/.ollama` içinde (named volume) — aynı volume yeniden mount edilir; indirme gerekmez. Risk yalnızca compose yerine elle `docker run` yapılırsa (yeni anonim volume) — plan compose ile, risk yok |
| **Mevcut index bozulur mu?** | **Hayır** | Qdrant'a (vektörler, payload, fingerprint) hiç dokunulmuyor. İleride yapılacak yeni embedding'ler GPU'da üretilir; CPU/GPU arasında olası mikro floating-point farkları `IndexSignature`'ı etkilemez (signature `ContentHash`+model adına dayalı, vektör değerine değil — worker.go:171) — diff mekanizması tutarlı kalır |
| **Ollama davranışı** | **Değişir (istenen)** | Model GPU'ya yüklenir; yanıtlar birebir aynı kalır (aynı model ağırlıkları), hız artar |

## 6. Doğrulama Planı (değişiklik sonrası)

| # | Komut | Başarı kriteri |
|---|---|---|
| 1 | `docker inspect arca-ollama --format '{{.HostConfig.DeviceRequests}}'` | `null` değil — nvidia device request görünür (örn. `[{"Driver":"nvidia","Count":1,...}]`) |
| 2 | `docker inspect arca-ollama --format '{{.HostConfig.Runtime}}'` | `nvidia` (device reservation ile Docker nvidia runtime'ı kullanır) |
| 3 | `curl -s localhost:11434/api/ps` (önce model yüklensin: `curl /api/embed` ile tek çağrı) | `size_vram > 0` (şu an `0.0 GB`) |
| 4 | Host'ta embed sırasında: `nvidia-smi --query-gpu=utilization.gpu,memory.used --format=csv` | embed çalışırken `utilization.gpu > 0` ve `memory.used` artıyor (şu an 0) |
| 5 | Tek embed gecikmesi: `curl -w '%{time_total}' /api/embeddings` (aynı metin, 3 ölçüm) | CPU ölçümüne göre belirgin hızlanma (CPU: ~0.2-0.5s; GPU: tek haneli ms-100ms bandı) |
| 6 | `docker compose ps` | `arca-ollama Up` — recreate sonrası servis sağlıklı; diğer servisler dokunulmamış (`Up 31 hours` → süre sıfırlanmaz) |
| 7 | ADR-0047 probe rep başına süre | 3017 chunk embed süresi CPU'daki ~10-20 dk'dan GPU'da dakika altına iner |

## 7. Notlar

- `docker exec arca-ollama nvidia-smi` **geçerli bir doğrulama değil** — ollama imajı `nvidia-smi` binary'si içermiyor (kanıt §2.2). GPU görünürlüğü §6/3-4 ile doğrulanır.
- WSL/Windows Docker kurulumunda GPU passthrough zaten çalışır durumda (host `nvidia-smi` RTX 4060 gösteriyor; daemon'da nvidia runtime kayıtlı) — patch'in ön koşulu sağlanmış.
- Bu değişiklik retrieval/embedding **davranışını değiştirmez** (aynı model, aynı girdi) — yalnızca inference hızını. ADR-0047 probe'u hızlandırır; benchmark sonuçlarının geçerliliğini etkilemez.
