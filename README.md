# netwatch

`netwatch` は Raspberry Pi 1 Model B 相当の古い Raspberry Pi を、有線 LAN 専用の軽量ネットワーク品質監視ノードとして使うための Go 製デーモンです。

絶対的な回線速度ではなく、共有型ネットワーク（マンションタイプの光回線など）で時間帯ごとの疎通品質の変化を継続的に記録することを主目的としています。

## 対象環境

- Raspberry Pi 1 Model B 相当
- Raspberry Pi OS Lite 32-bit
- 有線 LAN
- GUI なし
- `GOOS=linux GOARCH=arm GOARM=6` でクロスビルドした単一バイナリ

## 機能

- `fping` による gateway / external target の ping 測定
- Go 標準 resolver による DNS 応答時間の測定
- Go 標準 `net/http` + `httptrace` による HTTP 応答時間の測定
- 実サービス・ゲーム関連サービス・クラウドサービス入口 URL の低頻度 HTTP 監視
- HTTP 計測時の keep-alive 無効化と body 読み取り上限
- Cloudflare R2 公開ファイルを使った download probe
- packet loss と RTT min/avg/max の記録
- DNS duration と resolved IP の記録
- HTTP DNS/connect/TLS/TTFB/total time と status の記録
- HTTP body 読み取り bytes と truncate 状態の記録
- download bytes / duration / throughput Mbps の記録
- JSONL への append-only 保存
- JSONL の日次ローテーションと保持期間制御
- 起動時の JSONL 読み込み
- 最新状態 API
- target ごとの時系列 API
- service group ごとの最新状態・時系列・summary API
- 監視向け簡易ステータス API
- systemd 常駐用 unit

SQLite 保存はまだ未実装です。

## Raspberry Pi 側の必要パッケージ

```bash
sudo apt install -y fping curl dnsutils ethtool
```

daemon が直接利用する外部コマンドは `fping` です。
DNS / HTTP 計測は Go の標準ライブラリで実行します。
`curl` `dnsutils` `ethtool` は手動確認・トラブルシュート用途として含めています。

## 設定

設定ファイル例は [configs/netwatch.example.json](configs/netwatch.example.json) です。

```json
{
  "listen_addr": "0.0.0.0:8080",
  "data_dir": "/var/lib/netwatch",
  "data_file_pattern": "samples-%Y-%m-%d.jsonl",
  "retention_days": 14,
  "ping_interval_seconds": 30,
  "ping_count": 10,
  "ping_timeout_seconds": 15,
  "dns_interval_seconds": 60,
  "dns_timeout_seconds": 5,
  "http_interval_seconds": 60,
  "http_timeout_seconds": 10,
  "http_disable_keepalive": true,
  "http_max_body_bytes": 262144,
  "monitoring_thresholds": {
    "ping": {
      "gateway_rtt_avg_ms": {"warning": 5, "critical": 20},
      "gateway_loss_percent": {"warning": 0.1, "critical": 1},
      "external_rtt_avg_ms": {"warning": 100, "critical": 200},
      "external_loss_percent": {"warning": 1, "critical": 5}
    },
    "dns": {
      "duration_ms": {"warning": 300, "critical": 1000}
    },
    "http": {
      "total_ms": {"warning": 3000, "critical": 5000}
    },
    "download": {
      "r2_1mb_mbps": {"warning": 5, "critical": 1},
      "r2_10mb_mbps": {"warning": 10, "critical": 3}
    },
    "service": {
      "ok_rate_percent": {"warning": 95, "critical": 90}
    }
  },
  "download_probes": [
    {
      "name": "r2_1mb",
      "label": "R2 1MB",
      "display_order": 10,
      "url": "https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-1mb.bin",
      "expected_bytes": 1048576,
      "interval_seconds": 600,
      "timeout_seconds": 20,
      "enabled": true,
      "retry_on_alert": {
        "enabled": true,
        "intervals_seconds": [10, 30, 60, 180, 300, 600],
        "recovery_success_count": 2
      }
    },
    {
      "name": "r2_10mb",
      "label": "R2 10MB",
      "display_order": 20,
      "url": "https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-10mb.bin",
      "expected_bytes": 10485760,
      "interval_seconds": 3600,
      "timeout_seconds": 60,
      "enabled": true,
      "retry_on_alert": {
        "enabled": true,
        "intervals_seconds": [30, 60, 300, 600, 900, 1800, 3600],
        "recovery_success_count": 2
      }
    }
  ],
  "remote_speed_probes": [
    {
      "name": "scum_speedprobe",
      "label": "Scum Speedprobe",
      "display_order": 30,
      "url": "http://scum:8090/api/v1/speed/latest",
      "capabilities_url": "http://scum:8090/api/v1/capabilities",
      "health_url": "http://scum:8090/api/v1/health",
      "interval_seconds": 300,
      "timeout_seconds": 10,
      "enabled": false
    }
  ],
  "targets": [
    {
      "name": "gateway",
      "label": "Gateway",
      "display_order": 10,
      "type": "ping",
      "target": "192.168.1.1"
    },
    {
      "name": "cloudflare_dns",
      "label": "Cloudflare DNS",
      "display_order": 30,
      "type": "ping",
      "target": "1.1.1.1"
    },
    {
      "name": "google_dns",
      "label": "Google DNS",
      "display_order": 20,
      "type": "ping",
      "target": "8.8.8.8"
    },
    {
      "name": "google_dns_lookup",
      "type": "dns",
      "hostname": "www.google.com"
    },
    {
      "name": "cloudflare_home",
      "label": "Cloudflare Home",
      "display_order": 70,
      "type": "http",
      "group": "baseline",
      "category": "baseline",
      "url": "https://www.cloudflare.com/"
    },
    {
      "name": "google_generate_204",
      "label": "Google Generate 204",
      "display_order": 20,
      "type": "http",
      "group": "baseline",
      "category": "baseline",
      "url": "https://www.google.com/generate_204"
    },
    {
      "name": "youtube_home",
      "label": "YouTube Home",
      "display_order": 10,
      "type": "http",
      "group": "youtube",
      "category": "service",
      "url": "https://www.youtube.com/",
      "interval_seconds": 300
    },
    {
      "name": "docker_registry",
      "label": "Docker Registry",
      "display_order": 180,
      "type": "http",
      "group": "docker",
      "category": "container",
      "url": "https://registry-1.docker.io/v2/",
      "interval_seconds": 300,
      "expected_statuses": [200, 401]
    }
  ]
}
```

`targets[].type` は `ping`、`dns`、`http` に対応しています。
`http` target の method は Phase 3 では `GET` 固定です。`expected_statuses` を指定した場合は、その HTTP status code に含まれる応答だけを `ok: true` として扱います。未指定時は従来通り `200-399` を成功扱いにします。

`download_probes[]` は HTTP probe とは別に、大きな body を最後まで読み切って throughput を記録します。`enabled: false` の probe は実行されません。

`remote_speed_probes[]` は `netwatch-speedprobe` など別ノードの `/api/v1/speed/latest` を pull し、完了済み probe を `speedprobe` sample として保存します。`running` / `unknown` / `measured_at` なしの probe は保存しません。同じ `source + name + last_run_id` は重複保存しません。未導入環境では設定自体を省略するか、`enabled: false` のままにしてください。

`label` は API レスポンスの `display_name` として返す表示名です。未指定時は `name` から自動生成します。`display_order` は Viewer などの表示順に使う優先度です。`1` 以上の小さい値ほど前に並び、未指定または `0` の item は従来通り `name` 昇順で後ろに並びます。後から間に追加しやすいよう、初期設定では 10 刻みで指定しています。

`monitoring_thresholds` は `/api/monitoring/status`、`/api/monitoring/compact`、`/api/monitoring/thresholds` の返却値に使います。RTT / loss / duration / HTTP total は値が大きいほど悪い指標で、download Mbps と service ok rate は値が小さいほど悪い指標です。`download` 閾値は Core Network Status ではなく Throughput Status の判定に使います。不正な閾値は起動時の設定検証でエラーになります。

既存の単一ファイル設定も引き続き使えます。

```json
{
  "data_path": "/var/lib/netwatch/samples.jsonl"
}
```

`data_dir` を指定した場合は日次ローテーションを使います。デフォルトの保持期間は14日です。

```text
/var/lib/netwatch/samples-2026-06-06.jsonl
/var/lib/netwatch/samples-2026-06-07.jsonl
```

## 実サービス監視

Phase 3 以降の `type: "http"` は HTTP endpoint probe です。DNS 解決、TCP 接続、TLS handshake、HTTP 応答、応答時間を測ります。
ページ本文のスクレイピング、公式ステータスページのインシデント内容解析、大容量 download、ログインが必要な API の業務的な成功判定、実ゲームサーバー IP の探索は行いません。

実サービス target は次の metadata を持てます。

- `group`: 利用者が見たいまとまり。例: `youtube`, `netflix`, `slack`, `steam`, `aws`, `azure`, `psn`, `pcgame`
- `category`: 大分類。例: `service`, `cloud`, `game`, `baseline`
- `interval_seconds`: target 固有の測定間隔。未指定時は `http_interval_seconds`
- `timeout_seconds`: target 固有の timeout。未指定時は `http_timeout_seconds`
- `expected_statuses`: target 固有の成功 status code。未指定時は `200-399`

`category` を指定した HTTP target では `group` も必須です。既存の `group` なし HTTP target は後方互換のため読み込めます。

実サービス URL に負荷をかけないよう、サンプル target は `interval_seconds: 300` を基本にしています。高頻度監視は避けてください。

HTTP endpoint として到達できていることを確認したい target では、`401` / `403` なども `expected_statuses` に含められます。たとえば Docker Registry の `401`、OpenAI API の `401` / `403` は認証なしでは業務的には失敗ですが、host と endpoint への到達性確認としては成功扱いにできます。

Phase 3.5 では `sf6_buckler_info` をサンプル設定から外しています。実機確認で常時 `403` になり、ネットワーク不調ではなく相手側のアクセス拒否と判断しやすいためです。`pcgame` group 自体は維持し、`riot_status` / `ea_status` などを監視対象にしています。

HTTP 計測は時系列比較の安定性を優先し、デフォルトで keep-alive を無効化して毎回 DNS / TCP connect / TLS handshake を測ります。また、body はデフォルトで 256KiB まで読み取り、上限に達した場合も HTTP 応答自体が成功なら `ok: true` として扱います。

初期設定例には以下の group を含めています。

```text
youtube, netflix, slack, steam, aws, azure, psn, pcgame, github, openai, laravel, docker
```

## Status Page Monitor

`type: "http"` は、自宅ネットワークから指定 URL に到達できるかを測る endpoint probe です。`status_pages[]` はそれとは別に、事業者が公開している公式 status API を読み、provider が報告している障害、重要 component の劣化、未解決 incident、maintenance を `ok` / `warning` / `critical` / `unknown` に要約します。

このフェーズでは `provider: "statuspage"` の Statuspage summary API のみ対応しています。HTML scraping、RSS、Slack Status 独自 API、AWS Health 詳細 API などは対象外です。

```json
{
  "status_pages": [
    {
      "name": "github_status",
      "label": "GitHub Status",
      "display_order": 10,
      "type": "status_page",
      "provider": "statuspage",
      "group": "github",
      "category": "dev",
      "url": "https://www.githubstatus.com/api/v2/summary.json",
      "interval_seconds": 300,
      "important_components": [
        "Git Operations",
        "API Requests",
        "Webhooks",
        "Actions",
        "Packages"
      ]
    }
  ]
}
```

`important_components` は Statuspage API の `components[].name` と照合します。重要 component の `major_outage` は `critical`、`degraded_performance` / `partial_outage` / `under_maintenance` は `warning` として provider level に反映します。重要 component 以外の component は API response には保存・返却しますが、provider 全体の level には原則反映しません。

`status.indicator` は `none -> ok`、`minor -> warning`、`major` / `critical -> critical`、不明または欠落時は `unknown` です。未解決 incident がある場合と、進行中 maintenance がある場合は `warning` として扱います。

## Download Probe

Phase 5 では、Cloudflare R2 の public `r2.dev` URL に置いた固定サイズファイルを download し、`downloaded_bytes`、`duration_ms`、`bytes_per_sec`、`mbps` を記録します。

これは絶対的な回線速度測定ではなく、同じ Raspberry Pi、同じ経路、同じファイルを継続測定して時間帯ごとの傾向を見るための probe です。初代 Raspberry Pi の有線 LAN は 100Mbps 制約があり、CPU や USB/LAN 構成の影響も受けるため、結果は契約回線の最大速度とは一致しません。

当面は以下の R2 `r2.dev` public URL を使います。将来、Cloudflare 側の運用を変える場合はカスタムドメインに切り替える可能性があります。

```json
{
  "download_probes": [
    {
      "name": "r2_1mb",
      "label": "R2 1MB",
      "display_order": 10,
      "url": "https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-1mb.bin",
      "expected_bytes": 1048576,
      "interval_seconds": 600,
      "timeout_seconds": 20,
      "enabled": true,
      "retry_on_alert": {
        "enabled": true,
        "intervals_seconds": [10, 30, 60, 180, 300, 600],
        "recovery_success_count": 2
      }
    },
    {
      "name": "r2_10mb",
      "label": "R2 10MB",
      "display_order": 20,
      "url": "https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-10mb.bin",
      "expected_bytes": 10485760,
      "interval_seconds": 3600,
      "timeout_seconds": 60,
      "enabled": true,
      "retry_on_alert": {
        "enabled": true,
        "intervals_seconds": [30, 60, 300, 600, 900, 1800, 3600],
        "recovery_success_count": 2
      }
    }
  ]
}
```

Download Probe は通常時は低頻度で測定します。異常を検知した場合のみ短い間隔で再測定し、回復を早めに検知します。`retry_on_alert.enabled: false` または未指定の場合は従来どおり通常間隔だけで動きます。

Adaptive Retry:

- `r2_1mb`: normal `10m`, retry `10s, 30s, 1m, 3m, 5m, 10m`, recovery `2 consecutive OK`
- `r2_10mb`: normal `60m`, retry `30s, 1m, 5m, 10m, 15m, 30m, 60m`, recovery `2 consecutive OK`

測定頻度を高くしすぎると、測定自体が回線を消費します。特に `r2_10mb` は 10MB を取得するため、異常時でも 10 秒間隔にはせず、最短 30 秒から再測定します。

## ローカル実行

ローカルで `/var/lib/netwatch` に書けない場合は、設定ファイルの `data_path` を一時ディレクトリなどに変更してください。

```bash
go run ./cmd/netwatchd -config configs/netwatch.example.json
```

## ビルド

通常のローカルビルド:

```bash
make build
```

Raspberry Pi 1 向け ARMv6 クロスビルド:

```bash
make build-armv6
```

生成物:

```text
dist/netwatchd-linux-armv6
```

## Raspberry Pi への配置例

ビルド済みバイナリを Raspberry Pi に転送します。

```bash
scp dist/netwatchd-linux-armv6 pi@raspberrypi.local:/tmp/netwatchd-linux-armv6
```

Raspberry Pi 側で、リポジトリ一式を置いたディレクトリからインストールします。

```bash
sudo install -d /opt/netwatch
sudo cp -r . /opt/netwatch
cd /opt/netwatch
sudo scripts/install-raspi.sh /tmp/netwatchd-linux-armv6
```

`/etc/netwatch/netwatch.json` を環境に合わせて編集してから systemd を有効化します。

```bash
sudo systemctl enable --now netwatch
sudo systemctl status netwatch
```

systemd unit は [deploy/systemd/netwatch.service](deploy/systemd/netwatch.service) にあります。

## メンテナンスコマンド

`netwatch-jsonl` は保存済み JSONL を修正するための補助コマンドです。group 名を変更したい場合は、`rename-group` に旧 group 名と新 group 名を渡します。対象行以外と JSON として読めない行はそのまま保持し、実行時は変更したファイルごとにバックアップを作ります。

```bash
make build-armv6
scp dist/netwatch-jsonl-linux-armv6 pi@raspberrypi.local:/tmp/netwatch-jsonl-linux-armv6
ssh pi@raspberrypi.local
sudo install -m 0755 /tmp/netwatch-jsonl-linux-armv6 /usr/local/bin/netwatch-jsonl
```

実行前に dry-run で変更対象数を確認します。

```bash
sudo netwatch-jsonl rename-group -data-dir /var/lib/netwatch -dry-run pcgame pc_game
```

問題なければ `netwatch` を止めてから実行し、完了後に再起動します。

```bash
sudo systemctl stop netwatch
sudo netwatch-jsonl rename-group -data-dir /var/lib/netwatch pcgame pc_game
sudo systemctl start netwatch
```

単一ファイルだけを対象にする場合は `-path` を使います。

```bash
sudo netwatch-jsonl rename-group -path /var/lib/netwatch/samples-2026-06-09.jsonl pcgame pc_game
```

## API

ヘルスチェック:

```bash
curl http://127.0.0.1:8080/api/health
```

```json
{
  "ok": true,
  "service": "netwatch",
  "version": "dev"
}
```

全 target の最新結果:

```bash
curl http://127.0.0.1:8080/api/latest
```

```json
{
  "ping": [
    {
      "name": "cloudflare_dns",
      "display_name": "Cloudflare DNS",
      "display_order": 30,
      "target": "1.1.1.1",
      "ok": true,
      "loss_percent": 0.0,
      "rtt_avg_ms": 18.2
    }
  ],
  "dns": [
    {
      "name": "google_dns_lookup",
      "hostname": "www.google.com",
      "ok": true,
      "duration_ms": 14.8
    }
  ],
  "http": [
    {
      "name": "cloudflare_home",
      "display_name": "Cloudflare Home",
      "display_order": 70,
      "url": "https://www.cloudflare.com/",
      "ok": true,
      "http_status": 200,
      "total_ms": 132.4
    }
  ],
  "download": [
    {
      "name": "r2_1mb",
      "display_name": "R2 1MB",
      "display_order": 10,
      "url": "https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-1mb.bin",
      "expected_bytes": 1048576,
      "downloaded_bytes": 1048576,
      "duration_ms": 1000.0,
      "bytes_per_sec": 1048576.0,
      "mbps": 8.388608,
      "ok": true
    }
  ]
}
```

ping target の最新結果:

```bash
curl http://127.0.0.1:8080/api/ping/latest
```

DNS target の最新結果:

```bash
curl http://127.0.0.1:8080/api/dns/latest
```

HTTP target の最新結果:

```bash
curl http://127.0.0.1:8080/api/http/latest
```

Download probe の最新結果:

```bash
curl http://127.0.0.1:8080/api/download/latest
```

Remote speedprobe の最新結果:

```bash
curl http://127.0.0.1:8080/api/speedprobe/latest
curl "http://127.0.0.1:8080/api/speedprobe/series?source=scum_speedprobe&name=r2_10mb&range=24h"
curl "http://127.0.0.1:8080/api/speedprobe/series?source=scum_speedprobe&name=r2_10mb&range=24h&bucket=5m"
```

Provider status の最新状態:

```bash
curl http://127.0.0.1:8080/api/status-pages/latest
```

AI 解析用 export:

```bash
curl -OJ "http://127.0.0.1:8080/api/export/ai?range=7d"
curl -OJ "http://127.0.0.1:8080/api/export/ai?from=2026-06-01&to=2026-06-08"
```

`/api/export/ai` は `application/zip` を返します。`range` は `1d` / `7d` / `30d` に対応します。`from` / `to` は `YYYY-MM-DD` 形式で、両方を指定した場合は `from` 以上 `to` 未満の期間を出力します。ZIP には `manifest.json`、`README.md`、`analysis-prompt.md`、`targets.json`、`thresholds.json`、`summary.json`、`samples.jsonl` が含まれます。`targets.json` は監視対象のスナップショットだけを含み、設定ファイル全体や認証情報は出力しません。`speedprobe` sample が存在する場合は、外部 observer の throughput として export に含めます。

実サービス group の最新状態:

```bash
curl http://127.0.0.1:8080/api/services/latest
```

```json
{
  "services": [
    {
      "group": "youtube",
      "display_name": "Youtube",
      "category": "service",
      "status": "ok",
      "targets": [
        {
          "name": "youtube_home",
          "display_name": "YouTube Home",
          "display_order": 10,
          "url": "https://www.youtube.com/",
          "ok": true,
          "http_status": 200,
          "total_ms": 312.4,
          "dns_ms": 20.1,
          "connect_ms": 31.2,
          "tls_ms": 45.7,
          "ttfb_ms": 180.2
        }
      ]
    }
  ]
}
```

target ごとの時系列:

```bash
curl 'http://127.0.0.1:8080/api/ping/series?name=cloudflare_dns&range=24h'
curl 'http://127.0.0.1:8080/api/dns/series?name=google_dns_lookup&range=24h'
curl 'http://127.0.0.1:8080/api/http/series?name=cloudflare_home&range=24h'
curl 'http://127.0.0.1:8080/api/download/series?name=r2_1mb&range=24h'
curl 'http://127.0.0.1:8080/api/services/series?group=pcgame&range=24h'
curl 'http://127.0.0.1:8080/api/services/series?name=youtube_home&range=24h'
```

`range` は `1h`、`6h`、`24h`、`7d`、`14d` に対応しています。
`/api/services/series` では `group` と `name` を同時指定すると `400 Bad Request` を返します。

## Chart-ready API

Phase 4 では、Mac アプリ `NetwatchViewer` の Swift Charts 表示向けに、既存 series API へ `bucket` と `max_points` を追加しています。

`bucket` 未指定時は従来通り raw samples を返します。`bucket` を指定した場合だけ、`points` 配列中心の chart-ready response を返します。

対応 bucket:

```text
1m, 5m, 15m, 30m, 1h, 6h, 1d
```

`max_points` は chart points の最大数です。未指定時は `500`、指定可能範囲は `10` から `2000` です。範囲外や不正値は `400 Bad Request` を返します。
不正な `range` / `bucket` も `400 Bad Request` です。bucket 付き chart series で存在しない `name` / `group` は structured `404 Not Found` を返します。

Ping chart:

```bash
curl 'http://127.0.0.1:8080/api/ping/series?name=cloudflare_dns&range=24h&bucket=5m&max_points=300'
```

```json
{
  "type": "ping",
  "name": "cloudflare_dns",
  "display_name": "Cloudflare DNS",
  "target": "1.1.1.1",
  "range": "24h",
  "bucket": "5m",
  "points": [
    {
      "ts": "2026-06-06T00:00:00+09:00",
      "avg_ms": 12.3,
      "min_ms": 10.8,
      "max_ms": 20.1,
      "loss_percent": 0.0,
      "sample_count": 10
    }
  ]
}
```

DNS chart:

```bash
curl 'http://127.0.0.1:8080/api/dns/series?name=google_dns_lookup&range=24h&bucket=5m'
```

HTTP chart:

```bash
curl 'http://127.0.0.1:8080/api/http/series?name=youtube_home&range=24h&bucket=5m'
```

Download chart:

```bash
curl 'http://127.0.0.1:8080/api/download/series?name=r2_1mb&range=24h&bucket=5m&max_points=500'
```

```json
{
  "type": "download",
  "name": "r2_1mb",
  "display_name": "R2 1MB",
  "range": "24h",
  "bucket": "5m",
  "bucket_seconds": 300,
  "points": [
    {
      "ts": "2026-06-06T12:00:00+09:00",
      "avg_mbps": 31.4,
      "min_mbps": 24.8,
      "max_mbps": 40.1,
      "failure_count": 0,
      "sample_count": 1
    }
  ]
}
```

Services chart:

```bash
curl 'http://127.0.0.1:8080/api/services/series?group=pcgame&range=24h&bucket=5m'
```

Chart 集約仕様:

- Ping: `rtt_avg_ms` の平均、`rtt_min_ms` の最小、`rtt_max_ms` の最大、`sent` / `received` から再計算した `loss_percent`
- DNS: 成功 sample の `duration_ms` 平均/最小/最大、失敗 sample は `failure_count`
- HTTP: 成功 sample の `total_ms` / `ttfb_ms` 平均、`total_ms` 最大、失敗 sample は `failure_count` / `timeout_count`
- Download: 成功 sample の `mbps` 平均/最小/最大、失敗 sample は `failure_count` / `timeout_count`
- Services: group 内 HTTP sample の成功 `total_ms` 平均/最大、`ok_rate`、`failure_count`
- `ok=false` の DNS/HTTP/download sample は平均計算から除外し、失敗数には含めます
- bucket 内に成功 sample がない場合、平均値は JSON 上で省略され、`0ms` と欠損を区別します

Viewer 補助 API:

```bash
curl http://127.0.0.1:8080/api/charts/catalog
curl 'http://127.0.0.1:8080/api/charts/overview?range=24h&bucket=5m&max_points=500'
curl http://127.0.0.1:8080/api/monitoring/thresholds
curl http://127.0.0.1:8080/api/capabilities
```

`/api/charts/catalog` は、現在の設定から利用可能な chart target を返します。Mac アプリ側で `cloudflare_dns` や `pcgame` などを固定せず、API から発見するための endpoint です。

```json
{
  "generated_at": "2026-06-06T12:00:00+09:00",
  "timezone": "Asia/Tokyo",
  "defaults": {
    "range": "24h",
    "bucket": "5m",
    "max_points": 500
  },
  "supported": {
    "ranges": ["1h", "6h", "24h", "7d", "14d"],
    "buckets": ["1m", "5m", "15m", "30m", "1h", "6h", "1d"]
  },
  "ping": [],
  "dns": [],
  "http": [],
  "download": [
    {
      "name": "r2_1mb",
      "display_name": "R2 1MB",
      "display_order": 10,
      "url": "https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-1mb.bin",
      "expected_bytes": 1048576,
      "label": "R2 1MB"
    },
    {
      "name": "r2_10mb",
      "display_name": "R2 10MB",
      "display_order": 20,
      "url": "https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-10mb.bin",
      "expected_bytes": 10485760,
      "label": "R2 10MB"
    }
  ],
  "speedprobe": [
    {
      "name": "r2_10mb",
      "source": "scum_speedprobe",
      "display_name": "R2 10MB",
      "display_order": 30,
      "url": "https://example.com/netwatch-10mb.bin",
      "label": "R2 10MB"
    }
  ],
  "service_groups": []
}
```

`/api/charts/overview` は、代表的な ping / HTTP / download / speedprobe / service group の chart data をまとめて返します。speedprobe は source ごとに 10MB probe を優先し、なければ 100MB、それもなければ最初の probe を代表として返します。存在しない target や group はスキップします。

Chart response には以下の metadata が含まれます。

```json
{
  "generated_at": "2026-06-06T12:00:00+09:00",
  "actual_range_start": "2026-06-05T12:00:00+09:00",
  "actual_range_end": "2026-06-06T12:00:00+09:00",
  "timezone": "Asia/Tokyo",
  "range": "24h",
  "bucket": "5m",
  "bucket_seconds": 300,
  "max_points": 500
}
```

ゼロ値の扱い:

- `sample_count` は常に返します
- `failure_count` / `timeout_count` は `0` でも返します
- service group の `ok_rate` は正常時も `100` を返します
- 平均値が計算できない場合は `null` または省略で返し、`0ms` と欠損を混同しません

`/api/monitoring/thresholds` は、Viewer がグラフに警戒ラインを描くためのしきい値を返します。値は `/api/monitoring/status` の判定と同じです。

```json
{
  "generated_at": "2026-06-07T12:00:00+09:00",
  "ping": {
    "gateway_rtt_avg_ms": {"warning": 5, "critical": 20},
    "gateway_loss_percent": {"warning": 0.1, "critical": 1},
    "external_rtt_avg_ms": {"warning": 100, "critical": 200},
    "external_loss_percent": {"warning": 1, "critical": 5}
  },
  "dns": {
    "duration_ms": {"warning": 300, "critical": 1000}
  },
  "http": {
    "total_ms": {"warning": 3000, "critical": 5000}
  },
  "download": {
    "r2_1mb_mbps": {"warning": 5, "critical": 1},
    "r2_10mb_mbps": {"warning": 10, "critical": 3}
  },
  "service": {
    "ok_rate_percent": {"warning": 95, "critical": 90}
  }
}
```

`/api/capabilities` は、API version と利用可能な機能を返します。

```json
{
  "service": "netwatch",
  "version": "dev",
  "api_version": "0.4",
  "features": {
    "ping": true,
    "dns": true,
    "http": true,
    "download": true,
    "download_series": true,
    "speedprobe": true,
    "speedprobe_series": true,
    "charts": true,
    "charts_download": true,
    "charts_catalog": true,
    "charts_overview": true,
    "monitoring_status": true,
    "monitoring_thresholds": true,
    "monitoring_status_history": true,
    "monitoring_status_history_2h_5m": true,
    "monitoring_compact": true
  },
  "monitoring_status_history": {
    "ranges": ["1h", "2h", "6h", "24h", "7d"],
    "buckets": ["5m", "15m", "30m", "1h"]
  },
  "monitoring_compact": {
    "history_range": "2h",
    "history_bucket": "5m",
    "history_points": 24
  }
}
```

Chart API 系の不正 query は構造化エラーを返します。

```json
{
  "error": {
    "code": "invalid_max_points",
    "message": "max_points must be between 10 and 2000",
    "param": "max_points",
    "min": 10,
    "max": 2000
  }
}
```

実サービス group の summary:

```bash
curl 'http://127.0.0.1:8080/api/services/summary?range=24h'
curl 'http://127.0.0.1:8080/api/services/summary?group=pcgame&range=24h'
```

```json
{
  "range": "24h",
  "groups": [
    {
      "group": "pcgame",
      "category": "game",
      "sample_count": 864,
      "ok_count": 848,
      "ok_rate": 98.1,
      "avg_total_ms": 780.4,
      "max_total_ms": 4500.0,
      "avg_dns_ms": 24.5,
      "avg_connect_ms": 42.1,
      "avg_tls_ms": 68.3,
      "avg_ttfb_ms": 410.7,
      "max_ttfb_ms": 2100.0,
      "timeout_count": 6,
      "error_count": 8
    }
  ]
}
```

集計仕様:

- `sample_count`: 対象 group の HTTP sample 数
- `ok_count`: `ok: true` の sample 数
- `ok_rate`: `ok_count / sample_count * 100`
- `avg_total_ms`: `total_ms > 0` の sample だけで平均
- `max_total_ms`: `total_ms` の最大値
- `timeout_count`: error 文字列に `timeout` または `deadline exceeded` を含む sample 数
- `error_count`: `error` がある、または `ok: false` の sample 数
- `avg_dns_ms` / `avg_connect_ms` / `avg_tls_ms` / `avg_ttfb_ms`: 値が存在する sample だけで平均
- `sample_count = 0` の場合はゼロ除算せず、rate と平均値は `0` です

監視クライアント向けの簡易ステータス:

```bash
curl http://127.0.0.1:8080/api/monitoring/status
```

```json
{
  "alert": true,
  "source": "netwatch",
  "status_id": "warning-packet_loss-cloudflare_dns-a01b46b4",
  "generated_at": "2026-06-07T12:00:00+09:00",
  "level": "warning",
  "title": "NET WARNING",
  "message": "packet loss 2.0%",
  "primary_reason": {
    "code": "packet_loss",
    "level": "warning",
    "target": "cloudflare_dns",
    "metric": "loss_percent",
    "value": 2.0,
    "warning": 1,
    "critical": 5
  },
  "reasons": [
    {
      "code": "packet_loss",
      "level": "warning",
      "target": "cloudflare_dns",
      "metric": "loss_percent",
      "value": 2.0,
      "warning": 1,
      "critical": 5
    }
  ]
}
```

判定目安:

- `level` は reason の中に `critical` があれば `critical`、それ以外で reason があれば `warning`、reason がなければ `ok` です
- `status_id` は正常時 `ok`、異常時は level / primary reason / target / reason fingerprint から作る安定 ID です。同じ異常理由の継続中に毎回変わる timestamp 型 ID ではありません
- `primary_reason` は `critical` 優先、次に reason code 優先度、次に閾値超過率、最後に検出順で選ばれます
- monitoring status は Core Network Status として、ping / DNS のみを top-level の `level` / `alert` / `reasons` / `issue_count` に反映します。download_probes、speedprobe、HTTP Services、Status Page Monitor は収集・保存・個別 API には残りますが、Core Network Status の判定からは除外します
- Status Scope は `Core Network Status: Gateway RTT/loss, External RTT/loss, DNS`、`Throughput Status: legacy download_probes, speedprobe`、`Service Health: HTTP Services`、`Provider Status: Status Page monitors` です
- reason code 優先度は `gateway_loss`, `packet_loss`, `dns_failure`, `external_rtt_high`, `gateway_rtt_high`, `dns_slow` の順です
- `gateway` の packet loss が 0.1% 超なら `warning`、1% 以上なら `critical`
- `gateway` の RTT avg が 5ms 以上なら `warning`、20ms 以上なら `critical`
- external target の packet loss が 1% 以上なら `warning`
- external target の packet loss が 5% 以上なら `critical`
- external target の RTT avg が 200ms 以上なら `critical`
- external target の RTT avg が 100ms 以上なら `warning`
- DNS duration が 300ms 以上なら `warning`
- DNS duration が 1000ms 以上なら `critical`
- DNS failure は `warning`
- download failure は Throughput Status の `warning`
- `r2_1mb` が 5Mbps 未満なら Throughput Status の `warning`、1Mbps 未満なら `critical`
- `r2_10mb` が 10Mbps 未満なら Throughput Status の `warning`、3Mbps 未満なら `critical`
- Download throughput は Core Network Status には含めず、診断・可視化・AI 解析向けの Throughput Status として返します

Viewer の Status History では、過去の monitoring status を bucket 単位に集約する API を使います。

```bash
curl "http://127.0.0.1:8080/api/monitoring/status/history?range=24h&bucket=1h"
curl "http://127.0.0.1:8080/api/monitoring/status/history?range=2h&bucket=5m"
```

`range=24h&bucket=1h` と `range=2h&bucket=5m` はどちらも 24 個の point を返します。sample がない bucket も省略せず、`level: "unknown"` として返します。bucket の代表 `level` は `critical > warning > unknown > ok` の優先順位で決めます。

```json
{
  "source": "netwatch",
  "generated_at": "2026-06-07T13:30:00+09:00",
  "range": "24h",
  "bucket": "1h",
  "bucket_seconds": 3600,
  "actual_range_start": "2026-06-06T14:00:00+09:00",
  "actual_range_end": "2026-06-07T13:59:59+09:00",
  "points": [
    {
      "bucket_start": "2026-06-06T14:00:00+09:00",
      "bucket_end": "2026-06-06T14:59:59+09:00",
      "level": "ok",
      "alert": false,
      "sample_count": 12,
      "critical_count": 0,
      "warning_count": 0,
      "unknown_count": 0,
      "ok_count": 12
    }
  ],
  "summary": {
    "ok_count": 22,
    "warning_count": 1,
    "critical_count": 1,
    "unknown_count": 0
  }
}
```

M5Stack / macOS Widget / MenuBar などの小型表示向けには、現在状態と直近2時間の軽量履歴をまとめた compact API を使います。

```bash
curl "http://127.0.0.1:8080/api/monitoring/compact"
```

compact API の `label` は小型画面にそのまま表示する短い文字列です。

- `ok`: `NET OK`
- `warning`: `WARN`
- `critical`: `CRIT`
- `unknown`: `UNK`

top-level の `level` / `alert` / `issue_count` / `reasons` は Core Network Status です。`network_status` には同じ内容を構造化して返します。Download throughput は `throughput_status`、HTTP Services は `service_health`、Status Page Monitor は `provider_status` として別系統で返し、いずれも top-level の Core Network Status には反映しません。

`history` は `range=2h` / `bucket=5m` / 24 points 固定で、各 point は Core Network Status の `level` と `alert` のみを返します。`issue_count` は現在 status の reasons 数です。`throughput_status` は legacy download_probes と speedprobe の軽量要約で、`alert` は補足観測のため常に `false` です。`service_health` は HTTP service group の軽量 summary と warning / critical / unknown の issues を返します。`service_health.alert` も補足観測のため常に `false` です。`provider_status` は Status Page Monitor の軽量要約で、provider ごとの `level` と `description`、全体の `level` / `alert` / `issue_count` を返します。`provider_status` と `throughput_status` は top-level の Core Network Status とは独立しています。provider の `unknown` は `alert: false` で、`issue_count` に含めません。

```json
{
  "source": "netwatch",
  "generated_at": "2026-06-07T13:30:00+09:00",
  "level": "ok",
  "label": "NET OK",
  "alert": false,
  "title": "Network is stable",
  "message": "Core network probes are within thresholds.",
  "issue_count": 0,
  "primary_reason": null,
  "reasons": [],
  "network_status": {
    "level": "ok",
    "label": "NET OK",
    "alert": false,
    "title": "Network is stable",
    "message": "Core network probes are within thresholds.",
    "issue_count": 0,
    "reasons": []
  },
  "throughput_status": {
    "level": "warning",
    "alert": false,
    "issue_count": 1,
    "sources": [
      {
        "name": "legacy_download",
        "label": "Local Download Probes",
        "type": "download_probe",
        "level": "warning",
        "probes": [
          {
            "name": "r2_1mb",
            "label": "R2 1MB",
            "level": "warning",
            "status": "ok",
            "reason": "download_slow",
            "mbps": 3.2,
            "duration_ms": 2850,
            "expected_bytes": 1048576,
            "downloaded_bytes": 1048576,
            "measured_at": "2026-06-07T13:30:00+09:00"
          }
        ]
      }
    ]
  },
  "service_health": {
    "level": "warning",
    "alert": false,
    "issue_count": 1,
    "summary": [
      {
        "group": "openai",
        "label": "AI",
        "level": "warning",
        "ok": 1,
        "total": 2
      }
    ],
    "issues": [
      {
        "name": "chatgpt_home",
        "label": "ChatGPT Home",
        "group": "openai",
        "category": "ai",
        "level": "warning",
        "reason": "unexpected_status",
        "http_status_code": 503,
        "duration_ms": 1220,
        "measured_at": "2026-06-07T13:30:00+09:00"
      }
    ]
  },
  "provider_status": {
    "level": "ok",
    "alert": false,
    "issue_count": 0,
    "providers": []
  },
  "history": {
    "range": "2h",
    "bucket": "5m",
    "bucket_seconds": 300,
    "points": [
      {
        "level": "ok",
        "alert": false
      }
    ]
  }
}
```

## 手動確認コマンド

Raspberry Pi 上で daemon の外側から切り分ける場合は、以下を使います。

```bash
fping -C 5 -q 1.1.1.1
dig www.google.com
curl http://netpi:8080/api/health
curl http://netpi:8080/api/latest
curl http://netpi:8080/api/summary
curl -OJ "http://netpi:8080/api/export/ai?range=1d"
curl http://netpi:8080/api/services/latest
curl "http://netpi:8080/api/services/summary?range=1h"
curl http://netpi:8080/api/monitoring/status
curl "http://netpi:8080/api/monitoring/status/history?range=24h&bucket=1h"
curl "http://netpi:8080/api/monitoring/status/history?range=2h&bucket=5m"
curl "http://netpi:8080/api/monitoring/compact"
curl http://netpi:8080/api/charts/catalog
curl "http://netpi:8080/api/charts/overview?range=24h&bucket=5m&max_points=500"
curl http://netpi:8080/api/monitoring/thresholds
curl http://netpi:8080/api/capabilities
curl "http://netpi:8080/api/ping/series?name=cloudflare_dns&range=24h&bucket=5m"
curl "http://netpi:8080/api/http/series?name=youtube_home&range=24h&bucket=5m"
curl "http://netpi:8080/api/download/series?name=r2_1mb&range=24h&bucket=5m"
curl "http://netpi:8080/api/services/series?group=pcgame&range=24h&bucket=5m"
curl -o /dev/null -s -w "dns=%{time_namelookup} connect=%{time_connect} tls=%{time_appconnect} ttfb=%{time_starttransfer} total=%{time_total} code=%{http_code}\n" https://www.cloudflare.com/
curl -o /dev/null -s -w "dns=%{time_namelookup} connect=%{time_connect} tls=%{time_appconnect} ttfb=%{time_starttransfer} total=%{time_total} code=%{http_code}\n" https://www.youtube.com/
curl -o /dev/null -s -w "dns=%{time_namelookup} connect=%{time_connect} tls=%{time_appconnect} ttfb=%{time_starttransfer} total=%{time_total} code=%{http_code}\n" https://store.steampowered.com/
curl -o /dev/null -s -w "dns=%{time_namelookup} connect=%{time_connect} tls=%{time_appconnect} ttfb=%{time_starttransfer} total=%{time_total} code=%{http_code}\n" https://status.playstation.com/
curl -o /dev/null -L -w "size=%{size_download} total=%{time_total} speed=%{speed_download}\n" \
  https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-1mb.bin
curl -o /dev/null -L -w "size=%{size_download} total=%{time_total} speed=%{speed_download}\n" \
  https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-10mb.bin
```

R2 download の期待値は、1MB が `1048576` bytes、10MB が `10485760` bytes です。

## 開発用コマンド

```bash
make fmt
make test
make lint
make build
make build-armv6
```

`make lint` は `go vet ./...` を実行します。
golangci-lint は初回実装では導入していません。

## JSONL 形式

保存先は設定の `data_path` または `data_dir` / `data_file_pattern` で指定します。1 行 1 サンプルの JSONL です。

```json
{"ts":"2026-06-03T12:00:00+09:00","type":"ping","name":"cloudflare_dns","target":"1.1.1.1","sent":10,"received":10,"loss_percent":0.0,"rtt_min_ms":8.1,"rtt_avg_ms":10.4,"rtt_max_ms":16.9}
```

DNS:

```json
{"ts":"2026-06-04T12:00:00+09:00","type":"dns","name":"google_dns_lookup","hostname":"www.google.com","ok":true,"duration_ms":18.4,"resolved_ips":["142.250.207.100"]}
```

HTTP:

```json
{"ts":"2026-06-04T12:00:00+09:00","type":"http","name":"cloudflare_home","url":"https://www.cloudflare.com/","method":"GET","ok":true,"http_status":200,"dns_ms":12.3,"connect_ms":20.4,"tls_ms":41.7,"ttfb_ms":93.2,"total_ms":128.6}
```

実サービス HTTP:

```json
{"ts":"2026-06-04T12:00:00+09:00","type":"http","group":"youtube","category":"service","name":"youtube_home","url":"https://www.youtube.com/","method":"GET","ok":true,"http_status":200,"dns_ms":14.2,"connect_ms":20.1,"tls_ms":42.4,"ttfb_ms":120.5,"total_ms":210.2,"content_length_read":262144,"body_truncated":true}
```

Download:

```json
{"ts":"2026-06-06T12:00:00+09:00","type":"download","name":"r2_1mb","url":"https://pub-66e2ade26de745138962434a04cb1a46.r2.dev/netwatch-1mb.bin","expected_bytes":1048576,"downloaded_bytes":1048576,"duration_ms":1000,"bytes_per_sec":1048576,"mbps":8.388608,"ok":true,"retry_state":"normal","retry_attempt":0,"recovery_success_count":0,"next_check_at":"2026-06-06T12:10:00+09:00"}
```
