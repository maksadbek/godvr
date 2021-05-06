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
)

func main() {
	flag.Parse()

	settings := dvrip.Settings{
		Address: *address,
	}

	settings.SetDefaults()
	log.Printf("DEBUG: using the following settings: %+v", settings)

	conn, err := dvrip.New(settings)

	if err != nil {
		log.Panic(err)
	}

	err = conn.Login()
	if err != nil {
		log.Fatal("failed to login", err)
	}

	log.Printf("DEBUG: successfully logged in: %+v", settings)

	err = conn.SetKeepAlive()
	if err != nil {
		log.Fatal("failed to set keep alive:", err)
	}

	outChan := make(chan *dvrip.Frame)

	go func(chunkSize time.Duration) {
		var prevTime time.Time
		var videoFile, audioFile *os.File

		for frame := range outChan {
			log.Println(frame.Meta)

			now := time.Now()
			if chunkSize < now.Sub(prevTime) {
				err = syncAndClose(videoFile)
				if err != nil {
					panic(err)
				}

				err = syncAndClose(audioFile)
				if err != nil {
					panic(err)
				}

				err = os.MkdirAll(*outPath+"/"+(*name)+now.Format("/2006/01/02/"), os.ModePerm)
				if err != nil {
					panic(err)
				}

				videoFile, err = os.Create(*outPath + "/" + (*name) + now.Format("/2006/01/02/15.04.05.video"))
				if err != nil {
					panic(err)
				}

				audioFile, err = os.Create(*outPath + "/" + (*name) + now.Format("/2006/01/02/15.04.05.audio"))
				if err != nil {
					panic(err)
				}
			}

			if frame.Meta.Type == "G711A" {
				_, err = audioFile.Write(frame.Data)
				if err != nil {
					panic(err)
				}
			} else if frame.Meta.Frame != "" {
				videoFile.Write(frame.Data)
			} else {
				fmt.Println("WARNING: nor video or audio")
			}
		}

		syncAndClose(videoFile)
		syncAndClose(audioFile)
	}(*chunkInterval)

	err = conn.Monitor(*stream, outChan)
	if err != nil {
		log.Panic(err)
	}

	s := make(chan os.Signal)
	signal.Notify(s, os.Interrupt, os.Kill)

	select {
	case <-s:
		// TODO: gracefully stop
		log.Println("stopping")
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
