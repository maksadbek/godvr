package dvrip

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	portUDP = "34568"
	portTCP = "34567"
)

var magicEnd = [2]byte{0x0A, 0x00}

type statusCode int

const (
	statusOK                                  statusCode = 100
	statusUnknownError                        statusCode = 101
	statusUnsupportedVersion                  statusCode = 102
	statusRequestNotPermitted                 statusCode = 103
	statusUserAlreadyLoggedIn                 statusCode = 104
	statusUserIsNotLoggedIn                   statusCode = 105
	statusUsernameOrPasswordIsIncorrect       statusCode = 106
	statusUserDoesNotHaveNecessaryPermissions statusCode = 107
	statusPasswordIsIncorrect                 statusCode = 203
	statusStartOfUpgrade                      statusCode = 511
	statusUpgradeWasNotStarted                statusCode = 512
	statusUpgradeDataErrors                   statusCode = 513
	statusUpgradeError                        statusCode = 514
	statusUpgradeSuccessful                   statusCode = 515
)

var statusCodes = map[statusCode]string{
	statusOK:                                  "OK",
	statusUnknownError:                        "Unknown error",
	statusUnsupportedVersion:                  "Unsupported version",
	statusRequestNotPermitted:                 "Request not permitted",
	statusUserAlreadyLoggedIn:                 "User already logged in",
	statusUserIsNotLoggedIn:                   "User is not logged in",
	statusUsernameOrPasswordIsIncorrect:       "Username or password is incorrect",
	statusUserDoesNotHaveNecessaryPermissions: "User does not have necessary permissions",
	statusPasswordIsIncorrect:                 "Password is incorrect",
	statusStartOfUpgrade:                      "Start of upgrade",
	statusUpgradeWasNotStarted:                "Upgrade was not started",
	statusUpgradeDataErrors:                   "Upgrade data errors",
	statusUpgradeError:                        "Upgrade error",
	statusUpgradeSuccessful:                   "Upgrade successful",
}

type requestCode uint16

const (
	codeLogin            requestCode = 1000
	codeKeepAlive        requestCode = 1006
	codeSystemInfo       requestCode = 1020
	codeNetWorkNetCommon requestCode = 1042
	codeGeneral          requestCode = 1042
	codeChannelTitle     requestCode = 1046
	codeSystemFunction   requestCode = 1360
	codeEncodeCapability requestCode = 1360
	codeOPPTZControl     requestCode = 1400
	codeOPMonitor        requestCode = 1413
	codeOPTalk           requestCode = 1434
	codeOPTimeSetting    requestCode = 1450
	codeOPMachine        requestCode = 1450
	codeOPTimeQuery      requestCode = 1452
	codeAuthorityList    requestCode = 1470
	codeUsers            requestCode = 1472
	codeGroups           requestCode = 1474
	codeAddGroup         requestCode = 1476
	codeModifyGroup      requestCode = 1478
	codeDelGroup         requestCode = 1480
	codeAddUser          requestCode = 1482
	codeModifyUser       requestCode = 1484
	codeDelUser          requestCode = 1486
	codeModifyPassword   requestCode = 1488
	codeAlarmSet         requestCode = 1500
	codeOPNetAlarm       requestCode = 1506
	codeAlarmInfo        requestCode = 1504
	codeOPSendFile       requestCode = 1522
	codeOPSystemUpgrade  requestCode = 1525
	codeOPNetKeyboard    requestCode = 1550
	codeOPSNAP           requestCode = 1560
	codeOPMailTest       requestCode = 1636
)

var requestCodes = map[requestCode]string{
	codeOPMonitor: "OPMonitor",
}

var keyCodes = map[string]string{
	"M": "Menu",
	"I": "Info",
	"E": "Esc",
	"F": "Func",
	"S": "Shift",
	"L": "Left",
	"U": "Up",
	"R": "Right",
	"D": "Down",
}

type Conn struct {
	settings *Settings

	session        int32
	packetSequence int32
	aliveTime      time.Duration

	c     net.Conn
	cLock sync.Mutex

	stopMonitor chan struct{}
}

// Payload is a meta information about data that is going to be sent
type Payload struct {
	Head           byte
	Version        byte
	_              byte
	_              byte
	Session        int32
	SequenceNumber int32
	_              byte
	_              byte
	MsgID          int16
	BodyLength     int32
}

type MetaInfo struct {
	Width    int
	Height   int
	Datetime time.Time
	FPS      int
	Frame    string
	Type     string
}

type Frame struct {
	Data []byte
	Meta MetaInfo
}

type Settings struct {
	Network      string
	Address      string
	User         string
	Password     string
	PasswordHash string

	DialTimout   time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func (s *Settings) SetDefaults() {
	if s.User == "" {
		s.User = "admin"
	}

	if s.Network == "" {
		s.Network = "tcp"
	}

	if s.PasswordHash == "" {
		s.PasswordHash = sofiaHash(s.Password)
	}

	_, port, err := net.SplitHostPort(s.Address)
	if err != nil {
		switch s.Network {
		case "tcp":
			port = portTCP
		case "udp":
			port = portUDP
		default:
			panic("invalid network: " + s.Network)
		}

		s.Address += ":" + port
	}

	if s.DialTimout == 0 {
		s.DialTimout = time.Minute
	}

	if s.ReadTimeout == 0 {
		s.ReadTimeout = time.Minute
	}

	if s.WriteTimeout == 0 {
		s.WriteTimeout = time.Minute
	}
}

const alnum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func sofiaHash(password string) string {
	digest := md5.Sum([]byte(password))
	hash := make([]byte, 0, 8)

	for i := 1; i < len(digest); i += 2 {
		sum := int(digest[i-1]) + int(digest[i])
		hash = append(hash, alnum[sum%len(alnum)])
	}

	return string(hash)
}

func New(settings Settings) (*Conn, error) {
	conn := Conn{
		settings: &settings,
	}

	var err error

	conn.c, err = net.DialTimeout(settings.Network, settings.Address, settings.DialTimout)
	if err != nil {
		return nil, err
	}

	return &conn, nil
}

func (c *Conn) Login() error {
	body, err := json.Marshal(map[string]string{
		"EncryptType": "MD5",
		"LoginType":   "DVRIP-WEB",
		"PassWord":    c.settings.PasswordHash,
		"UserName":    c.settings.User,
	})

	if err != nil {
		return err
	}

	err = c.send(codeLogin, body)
	if err != nil {
		return err
	}

	_, resp, err := c.recv()
	if err != nil {
		return err
	}

	m := map[string]interface{}{}
	err = json.Unmarshal(resp, &m)
	if err != nil {
		return err
	}

	status, ok := m["Ret"].(float64)
	if !ok {
		return fmt.Errorf("ret is not an int: %v", m["Ret"])
	}

	if (statusCode(status) != statusOK) && (statusCode(status) != statusUpgradeSuccessful) {
		return fmt.Errorf("unexpected status code: %v - %v", status, statusCodes[statusCode(status)])
	}

	session, err := strconv.ParseUint(m["SessionID"].(string), 0, 32)
	if err != nil {
		return err
	}
	c.session = int32(session)
	c.aliveTime = time.Second * time.Duration(m["AliveInterval"].(float64))

	return nil
}

func (c *Conn) Command(command requestCode, data interface{}) (*Payload, []byte, error) {
	params, err := json.Marshal(map[string]interface{}{
		"Name":                requestCodes[command],
		"SessionID":           fmt.Sprintf("%08X", c.session),
		requestCodes[command]: data,
	})
	if err != nil {
		return nil, nil, err
	}

	c.cLock.Lock()
	defer c.cLock.Unlock()

	err = c.send(command, params)
	if err != nil {
		return nil, nil, err
	}

	resp, body, err := c.recv()

	return resp, body, err
}

func (c *Conn) StopMonitor() {
	println("stop monitor")
	c.stopMonitor <- struct{}{}
}

func (c *Conn) Monitor(stream string, ch chan *Frame) error {
	_, _, err := c.Command(codeOPMonitor, map[string]interface{}{
		"Action": "Claim",
		"Parameter": map[string]interface{}{
			"Channel":    0,
			"CombinMode": "NONE",
			"StreamType": stream,
			"TransMode":  "TCP",
		},
	})
	if err != nil {
		panic(err)
	}

	// TODO: check resp

	data, err := json.Marshal(map[string]interface{}{
		"Name":      "OPMonitor",
		"SessionID": fmt.Sprintf("%08X", c.session),
		"OPMonitor": map[string]interface{}{
			"Action": "Start",
			"Parameter": map[string]interface{}{
				"Channel":    0,
				"CombinMode": "NONE",
				"StreamType": stream,
				"TransMode":  "TCP",
			},
		},
	})

	c.cLock.Lock()
	defer c.cLock.Unlock()

	err = c.send(1410, data)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			frame, err := c.reassembleBinPayload()
			if err != nil {
				fmt.Println("error occurred", err)
				close(ch)
				return
			}

			select {
			case ch <- frame:
				println("got frame")
			case <-c.stopMonitor:
				println("got stop monitor chan")
				close(ch)
				return
			}
		}
	}()

	return nil
}

func (c *Conn) SetTime() error {
	_, _, err := c.Command(codeOPTimeSetting, []byte(time.Now().Format("2006-01-02 15:04:05")))

	return err
}

func (c *Conn) SetKeepAlive() error {
	body, err := json.Marshal(map[string]string{
		"Name":      "KeepAlive",
		"SessionID": fmt.Sprintf("%#08x", c.session),
	})

	if err != nil {
		return err
	}

	c.cLock.Lock()
	defer c.cLock.Unlock()

	err = c.send(codeKeepAlive, body)
	if err != nil {
		return err
	}

	_, _, err = c.recv()
	if err != nil {
		return err
	}

	//time.AfterFunc(c.aliveTime, func() {
	//	err := c.SetKeepAlive()
	//	if err != nil {
	//		panic(err) // TODO: panic or not ?
	//	}
	//})

	return nil
}

func (c *Conn) send(msgID requestCode, data []byte) error {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, Payload{
		Head:           255,
		Version:        0,
		Session:        c.session,
		SequenceNumber: c.packetSequence,
		MsgID:          int16(msgID),
		BodyLength:     int32(len(data)) + 2,
	}); err != nil {
		return err
	}

	err := binary.Write(&buf, binary.LittleEndian, data)
	if err != nil {
		return err
	}

	err = binary.Write(&buf, binary.LittleEndian, magicEnd)
	if err != nil {
		return err
	}

	_, err = c.c.Write(buf.Bytes())
	if err != nil {
		return err
	}

	return nil
}

func (c *Conn) recv() (*Payload, []byte, error) {
	var p Payload
	var b = make([]byte, 20)

	_, err := c.c.Read(b)
	if err != nil {
		return nil, nil, err
	}

	println("DEBUG: recv meta", b)

	err = binary.Read(bytes.NewReader(b), binary.LittleEndian, &p)
	if err != nil {
		return nil, nil, err
	}

	c.packetSequence += 1

	println("DEBUG: recv body length", p.BodyLength)

	body := make([]byte, p.BodyLength)
	err = binary.Read(c.c, binary.LittleEndian, &body)
	if err != nil {
		return nil, nil, err
	}

	println("DEBUG: recv body", body[:25])

	body = body[:len(body)-2] // skip the magic bytes

	return &p, body, nil
}

func (c *Conn) reassembleBinPayload() (*Frame, error) {
	var length int32 = 0
	var data bytes.Buffer
	var meta MetaInfo

	for {
		_, body, err := c.recv()
		if err != nil {
			return nil, err
		}

		buf := bytes.NewReader(body)

		if length == 0 {
			var dataType uint32
			err = binary.Read(buf, binary.BigEndian, &dataType)
			if err != nil {
				return nil, err
			}

			switch dataType {
			case 0x1FC, 0x1FE:
				frame := struct {
					Media    byte
					FPS      byte
					Width    byte
					Height   byte
					DateTime int32
					Length   int32
				}{}

				err = binary.Read(buf, binary.LittleEndian, &frame)
				if err != nil {
					return nil, err
				}

				if dataType == 0x1FC {
					meta.Frame = "I"
				}

				meta.Width = int(frame.Width) * 8
				meta.Height = int(frame.Height) * 8
				meta.Datetime = parseDatetime(int(frame.DateTime))
			case 0x1FD:
				// 4 bytes
				err = binary.Read(buf, binary.LittleEndian, &length)
				if err != nil {
					return nil, err
				}

				meta.Frame = "P"
			case 0x1FA, 0x1F9:
				packet := struct {
					Media      byte
					SampleRate byte
					Length     int32
				}{}

				err = binary.Read(buf, binary.LittleEndian, &packet)
				if err != nil {
					return nil, err
				}

				length = packet.Length
				meta.Type = parseMediaType(dataType, packet.Media)
			case 0xFFD8FFE0:
				return &Frame{
					Data: body,
					Meta: meta,
				}, nil
			default:
				return nil, fmt.Errorf("unexpected data type: %X", dataType)
			}
		}

		n, err := buf.WriteTo(&data)
		if err != nil {
			return nil, err
		}

		length -= int32(n)
		if length == 0 {
			frame := &Frame{
				Data: data.Bytes(),
				Meta: meta,
			}

			return frame, nil
		}
	}
}

func parseMediaType(dataType uint32, mediaCode byte) string {
	switch dataType {
	case 0x1FC, 0x1FD:
		switch mediaCode {
		case 1:
			return "MPEG4"
		case 2:
			return "H264"
		case 3:
			return "H265"
		}
	case 0x1F9:
		if mediaCode == 1 || mediaCode == 6 {
			return "info"
		}
	case 0x1FA:
		if mediaCode == 0xE {
			return "G711A"
		}
	case 0x1FE:
		if mediaCode == 0 {
			return "JPEG"
		}
	default:
		return "unknown"
	}

	return "unexpected"
}

func parseDatetime(value int) time.Time {
	second := value & 0x3F
	minute := (value & 0xFC0) >> 6
	hour := (value & 0x1F000) >> 12
	day := (value & 0x3E0000) >> 17
	month := (value & 0x3C00000) >> 22
	year := ((value & 0xFC000000) >> 26) + 2000

	return time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
}
