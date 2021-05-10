package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"godvr/internal/dvrip"
)

var (
	address       = flag.String("address", "192.168.1.147", "camera address: 192.168.1.147, 192.168.1.147:34567")
	name          = flag.String("name", "camera1", "name of the camera")
	outPath       = flag.String("out", "./", "output path that video files will be kept")
	chunkInterval = flag.Duration("chunkInterval", time.Minute*10, "time when application must create a new files")
	stream        = flag.String("stream", "Main", "camera stream name")
	user          = flag.String("user", "admin", "username")
	password      = flag.String("password", "password", "password")
)

func main() {
	flag.Parse()

	settings := dvrip.Settings{
		Address:  *address,
		User:     *user,
		Password: *password,
	}

	settings.SetDefaults()
	log.Printf("DEBUG: using the following settings: %+v", settings)

	conn, err := dvrip.New(settings)
	if err != nil {
		log.Panicf("failed to initiatiate connection: %v", err)
	}

again:
	err = conn.Login()
	if err != nil {
		log.Fatal("failed to login", err)
	}

	log.Print("DEBUG: successfully logged in")

	err = conn.SetKeepAlive()
	if err != nil {
		log.Fatal("failed to set keep alive:", err)
	}

	outChan := make(chan *dvrip.Frame)
	var videoFile, audioFile *os.File
	var errChan = make(chan error, 1)

	go func(chunkSize time.Duration) {
		prevTime := time.Now()
		videoFile, audioFile, err = createChunkFiles(time.Now())

		for frame := range outChan {
			log.Println(frame.Meta)
			now := time.Now()

			if chunkSize < now.Sub(prevTime) {
				err = syncAndClose(videoFile)
				if err != nil {
					fmt.Println("failed to sync and close file", err)
				}

				err = syncAndClose(audioFile)
				if err != nil {
					fmt.Println("failed to sync and close file", err)
				}

				videoFile, audioFile, err = createChunkFiles(now)
				prevTime = now
			}

			if frame.Meta.Type == "G711A" {
				_, err = audioFile.Write(frame.Data)
				if err != nil {
					fmt.Println("WARNING: failed to write to file", err)
				}
			} else if frame.Meta.Frame != "" {
				_, err = videoFile.Write(frame.Data)
				if err != nil {
					fmt.Println("WARNING: failed to write to file", err)
				}
			} else {
				fmt.Println("WARNING: nor video or audio")
			}
		}

		if conn.MonitorErr != nil {
			fmt.Println("error occurred", conn.MonitorErr)
			errChan <- conn.MonitorErr
		}
	}(*chunkInterval)

	err = conn.Monitor(*stream, outChan)
	if err != nil {
		log.Println("failed to start monitoring:", err)
	}

	s := make(chan os.Signal)
	signal.Notify(s, os.Interrupt, os.Kill)

	log.Println("listening interrupt")

	select {
	case <-errChan:
		log.Println("try again")
		goto again
	case <-s:
		log.Println("stopping")
		syncAndClose(videoFile)
		syncAndClose(audioFile)
		return
	}
}

func syncAndClose(f *os.File) error {
	err := f.Sync()
	if err != nil {
		return err
	}

	err = f.Close()
	if err != nil {
		return err
	}

	return nil
}

func createChunkFiles(t time.Time) (*os.File, *os.File, error) {
	err := os.MkdirAll(*outPath+"/"+(*name)+t.Format("/2006/01/02/"), os.ModePerm)
	if err != nil {
		return nil, nil, err
	}

	videoFile, err := os.Create(*outPath + "/" + (*name) + t.Format("/2006/01/02/15.04.05.video"))
	if err != nil {
		return nil, nil, err
	}

	audioFile, err := os.Create(*outPath + "/" + (*name) + t.Format("/2006/01/02/15.04.05.audio"))
	if err != nil {
		return nil, nil, err
	}

	return videoFile, audioFile, nil
}
