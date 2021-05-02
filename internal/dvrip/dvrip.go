package dvrip

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	portUDP = 34568
	portTCP = 34567
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

var requestCodes = map[requestCode]string{}

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
	settings Settings

	session        int32
	packetSequence int32
	aliveTime      int

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
	Body           []byte
	MagicEnd       [2]byte
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

func (s *Settings) validate() error {
	return nil
}

func (s *Settings) setDefaults() {

}

func New(settings Settings) (*Conn, error) {
	conn := Conn{
		settings: settings,
	}

	err := settings.validate()
	if err != nil {
		return nil, err
	}

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

	resp, err := c.send(codeLogin, body)
	if err != nil {
		return err
	}

	m := map[string]string{}
	err = json.Unmarshal(resp.Body, &m)
	if err != nil {
		return err
	}

	status, err := strconv.ParseUint(m["Ret"], 10, 16)
	if err != nil {
		return err
	}

	if statusCode(status) != statusOK || statusCode(status) != statusUpgradeSuccessful {
		return fmt.Errorf("unexpected status code: %v - %v", status, statusCodes[statusCode(status)])
	}

	session, err := strconv.ParseUint(m["session"], 10, 32)
	if err != nil {
		return err
	}
	c.session = int32(session)

	aliveInterval, err := strconv.ParseUint(m["AliveInterval"], 10, 32)
	if err != nil {
		return err
	}

	if err := c.setKeepAlive(time.Second * time.Duration(aliveInterval)); err != nil {
		panic(err)
	}

	return nil
}

func (c *Conn) Command(command requestCode, data []byte) (*Payload, error) {
	params, err := json.Marshal(map[string]string{
		"Name":                requestCodes[command],
		"SessionID":           fmt.Sprintf("%08X", c.session),
		requestCodes[command]: string(data),
	})
	if err != nil {
		return nil, err
	}

	resp, err := c.send(command, params)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

func (c *Conn) Monitor(stream string, ch chan *Frame) error {
	data, err := json.Marshal(map[string]interface{}{
		"Action": "Claim",
		"Parameter": map[string]interface{}{
			"Channel":     0,
			"CombineMode": "NONE",
			"StreamType":  stream,
			"TransMode":   "TCP",
		},
	})

	if err != nil {
		return err
	}

	_, err = c.Command(codeOPMonitor, data)
	if err != nil {
		panic(err)
	}

	// TODO: check resp

	data, err = json.Marshal(map[string]interface{}{
		"Name":      "OPMonitor",
		"SessionID": fmt.Sprintf("%08X", c.session),
		"OPMonitor": map[string]interface{}{
			"Action": "Start",
			"Parameter": map[string]interface{}{
				"Channel":     0,
				"CombineMode": "NONE",
				"StreamType":  stream,
				"TransMode":   "TCP",
			},
		},
	})

	_, err = c.send(1410, data)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			frame, err := c.reassembleBinPayload()
			if err != nil {
				println(err)
				return
			}

			select {
			case ch <- frame:
			case <-c.stopMonitor:
				close(ch)
				return
			}
		}
	}()

	return nil
}
func (c *Conn) setKeepAlive(aliveTime time.Duration) error {
	body, err := json.Marshal(map[string]string{
		"Name":      "KeepAlive",
		"SessionID": fmt.Sprintf("%08X", c.session),
	})

	if err != nil {
		return err
	}

	_, err = c.send(codeKeepAlive, body)
	if err != nil {
		return err
	}

	time.AfterFunc(aliveTime, func() {
		err := c.setKeepAlive(aliveTime)
		if err != nil {
			panic(err) // TODO: panic ?
		}
	})

	return nil
}

func (c *Conn) send(msgID requestCode, data []byte) (*Payload, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, Payload{
		Head:           255,
		Version:        0,
		Session:        c.session,
		SequenceNumber: c.packetSequence,
		MsgID:          int16(msgID),
		BodyLength:     int32(len(data)) + 2,
		Body:           data,
		MagicEnd:       magicEnd,
	}); err != nil {
		return nil, err
	}

	c.cLock.Lock()
	defer c.cLock.Lock()

	_, err := c.c.Write(buf.Bytes())
	if err != nil {
		return nil, err
	}

	resp, err := c.recv()
	return resp, err
}

func (c *Conn) recv() (*Payload, error) {
	var p Payload
	err := binary.Read(c.c, binary.LittleEndian, &p)
	if err != nil {
		return nil, err
	}

	c.packetSequence += 1

	return &p, nil
}

func (c *Conn) reassembleBinPayload() (*Frame, error) {
	var length int32 = 0

	for {
		p, err := c.recv()
		if err != nil {
			return nil, err
		}

		body := bytes.NewReader(p.Body)

		var meta MetaInfo

		if length == 0 {
			var dataType uint32
			err := binary.Read(body, binary.LittleEndian, &dataType)
			if err != nil {
				return nil, err
			}

			switch dataType {
			case 0x1FC, 0x1FE:
				// 12 bytes
				frame := struct {
					Media    byte
					FPS      byte
					Width    byte
					Height   byte
					DateTime int32
					Length   int32
				}{}

				err = binary.Read(body, binary.LittleEndian, &frame)
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
				err = binary.Read(body, binary.LittleEndian, &length)
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

				err = binary.Read(body, binary.LittleEndian, &packet)
				if err != nil {
					return nil, err
				}

				length = packet.Length
				meta.Type = parseMediaType(dataType, packet.Media)
			case 0xFFD8FFE0:
				var buf []byte
				_, err = body.Read(buf)
				return &Frame{
					Data: buf,
					Meta: meta,
				}, nil
			default:
				return nil, fmt.Errorf("unexpected data type: %X", dataType)
			}
		}

		var buf []byte
		n, err := body.Read(buf)
		if err != nil {
			return nil, err
		}

		length -= int32(n)
		if length == 0 {
			return &Frame{
				Data: buf,
				Meta: meta,
			}, nil
		}
	}
}

func parseMediaType(dataType uint32, mediaCode byte) string {
	switch dataType {
	case 0x1FC, 0x1FD:
		switch mediaCode {
		case 1:
			return "mpeg4"
		case 2:
			return "h264"
		case 3:
			return "h265"
		}
	case 0x1F9:
		if mediaCode == 1 || mediaCode == 6 {
			return "info"
		}
	case 0x1FA:
		if mediaCode == 0xE {
			return "g711a"
		}
	case 0x1FE:
		if mediaCode == 0 {
			return "jpeg"
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
