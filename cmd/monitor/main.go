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
	address = flag.String("address", "192.168.1.147", "camera address: 192.168.1.147, 192.168.1.147:34567")
	name    = flag.String("name", "camera1", "name of the camera")
	outPath = flag.String("out", "./", "output path that video files will be kept")
)

func main() {
	flag.Parse()

	conn, err := dvrip.New(dvrip.Settings{
		Address: *address,
	})

	if err != nil {
		panic(err)
	}

	err = conn.Login()
	if err != nil {
		panic(err)
	}

	err = conn.SetKeepAlive()
	if err != nil {
		log.Fatal("failed to set keep alive:", err)
	}

	outChan := make(chan *dvrip.Frame)

	go func(chunkSize time.Duration) {
		var prevTime time.Time
		var videoFile, audioFile *os.File

		for frame := range outChan {
			fmt.Println(frame.Meta)

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
				println("nor video or audio")
			}
		}

		syncAndClose(videoFile)
		syncAndClose(audioFile)
	}(time.Minute * 10) // create a new file every 10 minutes

	err = conn.Monitor("Main", outChan)
	if err != nil {
		panic(err)
	}

	s := make(chan os.Signal)
	signal.Notify(s, os.Interrupt, os.Kill)

	select {
	case <-s:
		// gracefully stop
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
