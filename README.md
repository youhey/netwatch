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
- packet loss と RTT min/avg/max の記録
- DNS duration と resolved IP の記録
- HTTP DNS/connect/TLS/TTFB/total time と status の記録
- JSONL への append-only 保存
- 起動時の JSONL 読み込み
- 最新状態 API
- target ごとの時系列 API
- 監視向け簡易ステータス API
- systemd 常駐用 unit

実サービス監視、download 速度計測、SQLite 保存はまだ未実装です。

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
  "data_path": "/var/lib/netwatch/samples.jsonl",
  "ping_interval_seconds": 30,
  "ping_count": 10,
  "ping_timeout_seconds": 15,
  "dns_interval_seconds": 60,
  "dns_timeout_seconds": 5,
  "http_interval_seconds": 60,
  "http_timeout_seconds": 10,
  "targets": [
    {
      "name": "gateway",
      "type": "ping",
      "target": "192.168.1.1"
    },
    {
      "name": "cloudflare_dns",
      "type": "ping",
      "target": "1.1.1.1"
    },
    {
      "name": "google_dns",
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
      "type": "http",
      "url": "https://www.cloudflare.com/"
    },
    {
      "name": "google_generate_204",
      "type": "http",
      "url": "https://www.google.com/generate_204"
    }
  ]
}
```

`targets[].type` は `ping`、`dns`、`http` に対応しています。
`http` target の method は Phase 2 では `GET` 固定です。

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
      "url": "https://www.cloudflare.com/",
      "ok": true,
      "http_status": 200,
      "total_ms": 132.4
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

target ごとの時系列:

```bash
curl 'http://127.0.0.1:8080/api/ping/series?name=cloudflare_dns&range=24h'
curl 'http://127.0.0.1:8080/api/dns/series?name=google_dns_lookup&range=24h'
curl 'http://127.0.0.1:8080/api/http/series?name=cloudflare_home&range=24h'
```

`range` は `1h`、`6h`、`24h`、`7d` に対応しています。

監視クライアント向けの簡易ステータス:

```bash
curl http://127.0.0.1:8080/api/monitoring/status
```

```json
{
  "alert": true,
  "source": "network",
  "level": "warning",
  "title": "NET SLOW",
  "message": "cloudflare_home http total 2500ms"
}
```

判定目安:

- external target の packet loss が 1% 以上なら `warning`
- external target の packet loss が 5% 以上なら `critical`
- external target の RTT avg が 100ms 以上なら `warning`
- DNS duration が 300ms 以上なら `warning`
- DNS failure は `warning`
- HTTP total が 2000ms 以上なら `warning`
- HTTP timeout / failure は `warning`
- `gateway` に packet loss がある場合は `critical`

## 手動確認コマンド

Raspberry Pi 上で daemon の外側から切り分ける場合は、以下を使います。

```bash
fping -C 5 -q 1.1.1.1
dig www.google.com
curl -o /dev/null -s -w "dns=%{time_namelookup} connect=%{time_connect} tls=%{time_appconnect} ttfb=%{time_starttransfer} total=%{time_total} code=%{http_code}\n" https://www.cloudflare.com/
```

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

保存先は設定の `data_path` で指定します。1 行 1 サンプルの JSONL です。

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
