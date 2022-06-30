// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Command livecaption pipes the stdin audio data to
// Google Speech API and outputs the transcript.
//
// As an example, gst-launch can be used to capture the mic input:
//
//    $ gst-launch-1.0 -v pulsesrc ! audioconvert ! audioresample ! audio/x-raw,channels=1,rate=16000 ! filesink location=/dev/stdout | livecaption
package main

// [START speech_transcribe_streaming_mic]
import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

	speech "cloud.google.com/go/speech/apiv1"
	rabbitmq "github.com/latonaio/rabbitmq-golang-client"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
)

// Pipe the stdin audio data to Google Speech API and outputs the transcript to RqbbitMQ.
// It uses Streaming Speech Recognition to process the stdin audio data and returns results in real time as the audio is processed.
func livecaption(ctx context.Context, mq *rabbitmq.RabbitmqClient, queueTo string) {
	client, err := speech.NewClient(ctx)
	if err != nil {
		log.Print(err)
		return
	}
	stream, err := client.StreamingRecognize(ctx)
	if err != nil {
		log.Print(err)
		return
	}
	// Send the initial configuration message.
	if err := stream.Send(&speechpb.StreamingRecognizeRequest{
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &speechpb.StreamingRecognitionConfig{
				Config: &speechpb.RecognitionConfig{
					Encoding:        speechpb.RecognitionConfig_LINEAR16,
					SampleRateHertz: 16000,
					LanguageCode:    "en-US",
				},
			},
		},
	}); err != nil {
		log.Print(err)
		return
	}

	// Pipe the stdin audio data to stdout.
	deviceNum := os.Getenv("DEVICE_NUMBER")
	cmd := exec.Command("bash", "-c", fmt.Sprintf("gst-launch-1.0 pulsesrc device=%s ! audioconvert ! audioresample ! audio/x-raw,channels=1,rate=16000 ! filesink location=/dev/stdout", deviceNum))
	stdout, _ := cmd.StdoutPipe()
	cmd.Start()
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	go func() {
		// Pipe stdin to the API.
		buf := make([]byte, 1024)
		for {
			select {
			case <-ctx.Done():
				log.Printf("canceled!")
				return
			default:
				n, err := stdout.Read(buf)
				if n > 0 {
					if err := stream.Send(&speechpb.StreamingRecognizeRequest{
						StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
							AudioContent: buf[:n],
						},
					}); err != nil {
						log.Printf("Could not send audio: %v", err)
					}
				}
				if err == io.EOF {
					// Nothing else to pipe, close the stream.
					if err := stream.CloseSend(); err != nil {
						log.Printf("Could not close stream: %v", err)
						return
					}
					return
				}
				if err != nil {
					log.Printf("Could not read from stdin: %v", err)
					continue
				}
			}
		}
	}()

	// Send transcripts to QUEUE_TO.
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Cannot stream results: %v", err)
			break
		}
		if err := resp.Error; err != nil {
			// Workaround while the API doesn't give a more informative error.
			if err.Code == 3 || err.Code == 11 {
				log.Print("WARNING: Speech recognition request exceeded limit of 60 seconds.")
			}
			log.Printf("Could not recognize: %v", err)
			break
		}
		for _, result := range resp.Results {
			for _, alt := range result.Alternatives {
				log.Printf("\"%v\" (confidence=%3f)\n", alt.Transcript, alt.Confidence)
				payload := map[string]interface{}{
					"transcript": alt.Transcript,
					"confidence": alt.Confidence,
				}
				if err := mq.Send(queueTo, payload); err != nil {
					log.Printf("error: %v", err)
				}
			}
		}
	}
}

func main() {
	// RabbitMQ connecting.
	log.Printf("started")

	url := os.Getenv("RABBITMQ_URL")
	queueFrom := os.Getenv("QUEUE_ORIGIN")
	queueTo := os.Getenv("QUEUE_TO")

	mq, err := rabbitmq.NewRabbitmqClient(
		url,
		[]string{queueFrom},
		[]string{queueTo},
	)
	if err != nil {
		log.Printf("failed to create RabbitmqClient: %v", err)
		return
	}
	log.Printf("connected!")
	defer mq.Close()

	iter, err := mq.Iterator()
	if err != nil {
		log.Printf("failed to create iterator: %v", err)
		return
	}
	defer mq.Stop()

	// If the microservice is running, the "isStart" flag must be true.
	// If the microservice is not stopping, the "isStop" flag must be true.
	isStart := false
	isStop := true

	ctx, cancel := context.WithCancel(context.Background())

	// Receive message from QUEUE_ORIGIN.
	for msg := range iter {
		log.Printf("received from: %v", msg.QueueName())
		log.Printf("data: %v", msg.Data())

		// Checks if the key and value of the message sent from RabbitMQ are corect.
		if val, ok := msg.Data()["flag"]; ok {
			if val == "start" && !isStart {
				log.Printf("Start!")
				isStart = true
				isStop = false
				go livecaption(ctx, mq, queueTo)
			} else if val == "stop" && !isStop {
				log.Printf("Stop!")
				isStop = true
				isStart = false
				cancel()
				ctx, cancel = context.WithCancel(context.Background())
			} else if val == "start" && isStart {
				log.Printf("Already started!")
			} else if val == "stop" && isStop {
				log.Printf("Already stopped!")
			} else {
				log.Printf("%s has an incorrect value!", msg.Data())
			}
		} else {
			log.Printf("%s has an incorrect key name!", msg.Data())
		}

		msg.Success()
	}

}

// [END speech_transcribe_streaming_mic]

