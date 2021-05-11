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
	retryTime     = flag.Duration("retryTime", time.Minute, "retry to connect if problem occur")
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

	for {
		err := monitor(settings)
		if err == nil {
			break
		}

		log.Print("fatal error: ", err)
		log.Printf("wait %v and try again", *retryTime)

		time.Sleep(*retryTime)
	}
}

func monitor(settings dvrip.Settings) error {
	conn, err := dvrip.New(settings)
	if err != nil {
		log.Printf("failed to initiatiate connection: %v", err)
		return err
	}

	err = conn.Login()
	if err != nil {
		log.Panic("failed to login", err)
		return err
	}

	log.Print("DEBUG: successfully logged in")

	err = conn.SetKeepAlive()
	if err != nil {
		log.Print("failed to set keep alive:", err)
		return err
	}

	outChan := make(chan *dvrip.Frame)
	var videoFile, audioFile *os.File

	err = conn.Monitor(*stream, outChan)
	if err != nil {
		log.Print("failed to start monitoring:", err)
		return err
	}

	stop := make(chan os.Signal)
	signal.Notify(stop, os.Interrupt, os.Kill)

	videoFile, audioFile, err = createChunkFiles(time.Now())
	if err != nil {
		return err
	}

	prevTime := time.Now()

	for {
		select {
		case frame, ok := <-outChan:
			if !ok {
				return conn.MonitorErr
			}

			now := time.Now()

			if prevTime.Add(*chunkInterval).After(now) {
				errs := syncClose(videoFile, audioFile)
				if err != nil {
					log.Printf("error occurred: %v", errs)
				}

				videoFile, audioFile, err = createChunkFiles(now)
				prevTime = now
			}

			err = processFrame(frame, audioFile, videoFile)
			if err != nil {
				log.Println("failed to process the frame", err)
				return err
			}
		case <-stop:
			log.Println("received interrupt signal: stopping")

			errs := syncClose(videoFile, audioFile)
			if err != nil {
				log.Printf("error occurred: %v", errs)
			}

			return nil
		}
	}

	return nil
}

func processFrame(frame *dvrip.Frame, audioFile, videoFile *os.File) error {
	log.Println("received frame with meta info:", frame.Meta)

	if frame.Meta.Type == "G711A" { // audio
		_, err := audioFile.Write(frame.Data)
		if err != nil {
			log.Println("WARNING: failed to write to file", err)
		}

		return nil
	}

	if frame.Meta.Frame != "" {
		_, err := videoFile.Write(frame.Data)
		if err != nil {
			log.Println("WARNING: failed to write to file", err)
		}

		return nil
	}

	return nil // TODO
}

func syncClose(files ...*os.File) (errs []error) {
	for _, f := range files {
		err := f.Sync()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to sync file: %v cause: %v", f.Name(), err))
		}

		err = f.Close()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to close file: %v cause: %v", f.Name(), err))
		}
	}

	return
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
