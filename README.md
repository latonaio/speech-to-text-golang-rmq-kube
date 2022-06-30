# speech-to-text-golang-rmq-kube
speech-to-text-golang-rmq-kubeは、Google Cloud Speech-to-Text APIをKubernetes上、Golangランタイムで動かすための手順概要です。  
音声を変換したテキストは、RabbitMQに送信されます。

## 動作環境
- Ubuntu (18.04 LTS)
- Docker
- Kubernetes
- RabbitMQ
- Go 1.18.3

## 動作手順
### サービスアカウントのJSONキーの作成
[Google Cloud Speech-to-Text](https://cloud.google.com/speech-to-text/docs/before-you-begin?hl=ja)を参考にサービスアカウントのJSONキーを作成し、カレントディレクトリにおいてください。  
ここでは、ダミーとして`your-project-credentials.json`が置いてあります。

### 環境変数・パスの書き換え
`deployment.yaml`の環境変数とパスを書き換えます。

- env:
	- GOOGLE_APPLICATION_CREDENTIALS：サービスアカウントキーのパス（カレントディレクトリに置く場合`/app/mnt/your-project-credentials.json`）
	- RABBITMQ_URL：RabbitMQのURL
	- QUEUE_ORIGIN：startまたはstopのフラグが送られてくるキュー名
	- QUEUE_TO：音声から変換されたテキストを送るキュー
	- DEVICE_NUMBER：マイクのSource number（`pactl list sources`のコマンドでSource一覧確認できる。）
- volume:
	- current-dir：カレントディレクトリのパス
	- pulse-socket：pulseaudioのソケットがあるパス
	- pulse-cookie：pulseaudioのCOOKIEがあるパス

### Dockerイメージのビルド
以下のコマンドで、Dockerイメージをビルドします。
```
bash docker-build.sh
```

### Kubernetes上で実行
以下のコマンドで、Deploymentを作成し、デーモンを起動します。
```
kubectl apply -f deployment.yaml
```

### Speech-to-Textの開始
QUEUE_ORIGINに対し、`{"flag":"start"}`を送ると、Speech-to-Textのマイクロサービスが開始します。  
入力された音声を変換したテキストはQUEUE_TOに送られます。

### Speech-to-Textの停止
QUEUE_ORIGINに対し、`{"flag":"stop"}`を送るとSpeech-to-Textのマイクロサービスが停止します。
