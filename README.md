# godvr

## Usage

```
$ make
$ ./monitor -help
Usage of ./monitor:
  -address string
    	camera address: 192.168.1.147, 192.168.1.147:34567 (default "192.168.1.147")
  -chunkInterval duration
    	time when application must create a new files (default 10m0s)
  -name string
    	name of the camera (default "camera1")
  -out string
    	output path that video files will be kept (default "./")
  -password string
    	password (default "password")
  -retryTime duration
    	retry to connect if problem occur (default 1m0s)
  -stream string
    	camera stream name (default "Main")
  -user string
    	username (default "admin")
$ ./monitor -debug -address 192.168.1.147 -name camera1 -out /recordings

```

> The valid way of setting debug mode is the following: `./monitor -debug` or `./monitor -debug=true`
> But not this: `./monitor -debug true`, see https://pkg.go.dev/flag#hdr-Command_line_flag_syntax