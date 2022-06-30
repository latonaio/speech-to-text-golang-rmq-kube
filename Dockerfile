FROM golang:1.18.3
RUN apt update && apt install -y libgstreamer1.0-0 gstreamer1.0-tools gstreamer1.0-pulseaudio gstreamer1.0-plugins-base-apps pulseaudio
RUN mkdir -p /app/mnt /app/src
COPY ["main.go", "go.mod", "go.sum", "/app/src"]
WORKDIR /app/src
RUN go mod download
RUN go build main.go
CMD bash
