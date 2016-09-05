package main

import (
	"encoding"
	"github.com/hybridgroup/gobot"
	"github.com/hybridgroup/gobot/platforms/gpio"
	"github.com/hybridgroup/gobot/platforms/raspi"
	"gopkg.in/lifx-tools/controlifx.v1"
	"gopkg.in/lifx-tools/implifx.v1"
	"log"
	"net"
	"sync"
	"time"
)

type writer func(always bool, t uint16, msg encoding.BinaryMarshaler) error

var (
	bulb struct {
		service             int8
		port                uint16
		time                int64
		resetSwitchPosition uint8
		dummyLoadOn         bool
		hostInfo            struct {
			signal         float32
			tx             uint32
			rx             uint32
			mcuTemperature uint16
		}
		hostFirmware struct {
			build   int64
			install int64
			version uint32
		}
		wifiInfo struct {
			signal         float32
			tx             uint32
			rx             uint32
			mcuTemperature int16
		}
		wifiFirmware struct {
			build   int64
			install int64
			version uint32
		}
		powerLevel uint16
		label      string
		tags       struct {
			tags  int64
			label string
		}
		version struct {
			vendor  uint32
			product uint32
			version uint32
		}
		info struct {
			time     int64
			uptime   int64
			downtime int64
		}
		mcuRailVoltage  uint32
		factoryTestMode struct {
			on       bool
			disabled bool
		}
		site     [6]byte
		location struct {
			location  [16]byte
			label     string
			updatedAt int64
		}
		group struct {
			group     [16]byte
			label     string
			updatedAt int64
		}
		owner struct {
			owner     [16]byte
			label     string
			updatedAt int64
		}
		state struct {
			color controlifx.HSBK
			dim   int16
			label string
			tags  uint64
		}
		lightRailVoltage  uint32
		lightTemperature  int16
		lightSimpleEvents []struct {
			time     int64
			power    uint16
			duration uint32
			waveform int8
			max      uint16
		}
		wanStatus  int8
		wanAuthKey [32]byte
		wanHost    struct {
			host               string
			insecureSkipVerify bool
		}
		wifi struct {
			networkInterface int8
			status           int8
		}
		wifiAccessPoints struct {
			networkInterface int8
			ssid             string
			security         int8
			strength         int16
			channel          uint16
		}
		wifiAccessPoint struct {
			networkInterface int8
			ssid             string
			pass             string
			security         int8
		}
		sensorAmbientLightLux float32
		sensorDimmerVoltage   uint32

		// Extra.
		startTime int64
	}

	ledDriver  *gpio.RgbLedDriver
	colorMutex sync.Mutex

	// Power.
	bLast int32

	// Current.
	hCurrent, sCurrent, bCurrent, kCurrent,
	// Start.
	hStart, sStart, bStart, kStart,
	// End.
	hEnd, sEnd, bEnd, kEnd,
	// Change.
	hChange, sChange, bChange, kChange int32

	// Duration.
	durationStart, duration int64

	actionCh = make(chan interface{})
)

type (
	PowerAction uint32

	ColorAction struct {
		Color    controlifx.HSBK
		Duration uint32
	}
)

func main() {
	gbot := gobot.NewGobot()
	raspiAdapter := raspi.NewRaspiAdaptor("raspi")
	ledDriver = gpio.NewRgbLedDriver(raspiAdapter, "led", "3", "5", "7")
	robot := gobot.NewRobot("ledBot",
		[]gobot.Connection{raspiAdapter},
		[]gobot.Device{ledDriver},
		func() {
			if err := Start(); err != nil {
				log.Fatalln(err)
			}
		},
	)

	gbot.AddRobot(robot)

	if errs := gbot.Start(); errs != nil {
		log.Fatalln(errs)
	}
}

func Start() error {
	// Connect.
	conn, err := implifx.Listen("0.0.0.0")
	if err != nil {
		return err
	}
	defer conn.Close()

	// Mock MAC.
	conn.Mac = 0xd0738f86bfaf

	configureBulb(conn.Port())

	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := updateLeds(); err != nil {
					log.Fatalln(err)
				}
			case action := <-actionCh:
				now := time.Now()

				colorMutex.Lock()

				switch action.(type) {
				case PowerAction:
					durationStart = now.UnixNano()
					duration = durationToNano(uint32(action.(PowerAction)))

					if bulb.powerLevel == 0xffff {
						bStart = bCurrent
						bEnd = bLast
					} else {
						bLast = bCurrent
						bStart = bCurrent
						bEnd = 0
					}

					bChange = bEnd - bStart
				case ColorAction:
					colorAction := action.(ColorAction)
					color := colorAction.Color

					hStart = hCurrent
					sStart = sCurrent
					bStart = bCurrent
					kStart = kCurrent
					hEnd = int32(color.Hue)
					sEnd = int32(color.Saturation)
					bEnd = int32(color.Brightness)
					kEnd = int32(color.Kelvin)
					hChange = hEnd - hStart
					sChange = sEnd - sStart
					bChange = bEnd - bStart
					kChange = kEnd - kStart

					// Hue change takes the shortest distance.
					if abs(hChange) > 0xffff/2 {
						if hChange > 0 {
							hChange -= 0xffff
						} else {
							hChange += 0xffff
						}
					}

					durationStart = now.UnixNano()
					duration = durationToNano(colorAction.Duration)
				}

				colorMutex.Unlock()
			}
		}
	}()

	for {
		n, raddr, recMsg, err := conn.Receive()
		if err != nil {
			if err.(net.Error).Temporary() {
				continue
			}
			return err
		}

		bulb.wifiInfo.rx += uint32(n)

		if err := handle(recMsg, func(always bool, t uint16, payload encoding.BinaryMarshaler) error {
			tx, err := conn.Respond(always, raddr, recMsg, t, payload)
			bulb.wifiInfo.tx += uint32(tx)

			return err
		}); err != nil {
			log.Println(err)
		}
	}
}

func updateLeds() error {
	if hCurrent == hEnd && sCurrent == sEnd && bCurrent == bEnd && kCurrent == kEnd {
		return nil
	}

	now := time.Now().UnixNano()

	colorMutex.Lock()
	defer colorMutex.Unlock()

	// Hue, saturation, and Kelvin linear interpolation.
	if now < durationStart+duration {
		hCurrent = lerp(durationStart, duration, now, hStart, hChange)
		sCurrent = lerp(durationStart, duration, now, sStart, sChange)
		bCurrent = lerp(durationStart, duration, now, bStart, bChange)
		kCurrent = lerp(durationStart, duration, now, kStart, kChange)
	} else {
		hCurrent = hEnd
		sCurrent = sEnd
		bCurrent = bEnd
		kCurrent = kEnd
	}

	r, g, b := hslToRgb(float32(hCurrent)/0xffff,
		float32(sCurrent)/0xffff,
		float32(bCurrent)/0xffff/2)

	if kCurrent >= 2500 && kCurrent <= 9000 {
		rK, gK, bK := kToRgb(float32(kCurrent))
		r *= rK
		g *= gK
		b *= bK
	}

	return ledDriver.SetRGB(byte(r*0xff), byte(g*0xff), byte(b*0xff))
}

func configureBulb(port uint16) {
	bulb.service = controlifx.UdpService
	bulb.port = port

	// Mock HostFirmware.
	bulb.hostFirmware.build = 1467178139000000000
	bulb.hostFirmware.version = 1968197120

	// Mock WifiInfo.
	bulb.wifiInfo.signal = 1e-5

	// Mock WifiFirmware.
	bulb.wifiFirmware.build = 1456093684000000000

	// Mock label.
	bulb.label = "Bias LED"

	bulb.version.vendor = controlifx.Color1000VendorId
	bulb.version.product = controlifx.Color1000ProductId

	// Mock location.
	bulb.location.location = [16]byte{187, 252, 158, 222, 71, 45, 6, 41, 96,
		22, 178, 149, 88, 166, 163, 213}
	bulb.location.label = "Home"
	bulb.location.updatedAt = 1471914564177000000

	// Mock group.
	bulb.group.group = [16]byte{99, 143, 185, 25, 104, 165, 213, 222, 97,
		64, 189, 203, 251, 16, 207, 11}
	bulb.group.label = "Bedroom"
	bulb.group.updatedAt = 1471914564104000000

	// Mock owner.
	bulb.owner.owner = [16]byte{48, 174, 196, 196, 45, 149, 64, 239, 165,
		207, 65, 146, 54, 50, 147, 44}
	bulb.owner.updatedAt = 1471914564298000000

	bulb.state.color.Kelvin = 3500

	// Extra.
	bulb.startTime = time.Now().UnixNano()
}

func handle(msg implifx.ReceivableLanMessage, w writer) error {
	switch msg.Header.ProtocolHeader.Type {
	case controlifx.GetServiceType:
		return getService(w)
	case controlifx.GetHostInfoType:
		return getHostInfo(w)
	case controlifx.GetHostFirmwareType:
		return getHostFirmware(w)
	case controlifx.GetWifiInfoType:
		return getWifiInfo(w)
	case controlifx.GetWifiFirmwareType:
		return getWifiFirmware(w)
	case controlifx.GetPowerType:
		return getPower(w)
	case controlifx.SetPowerType:
		return setPower(msg, w)
	case controlifx.GetLabelType:
		return getLabel(w)
	case controlifx.SetLabelType:
		return setLabel(msg, w)
	case controlifx.GetVersionType:
		return getVersion(w)
	case controlifx.GetInfoType:
		return getInfo(w)
	case controlifx.GetLocationType:
		return getLocation(w)
	case controlifx.GetGroupType:
		return getGroup(w)
	case controlifx.GetOwnerType:
		return getOwner(w)
	case controlifx.SetOwnerType:
		return setOwner(msg, w)
	case controlifx.EchoRequestType:
		return echoRequest(msg, w)
	case controlifx.LightGetType:
		return lightGet(w)
	case controlifx.LightSetColorType:
		return lightSetColor(msg, w)
	case controlifx.LightGetPowerType:
		return lightGetPower(w)
	case controlifx.LightSetPowerType:
		return lightSetPower(msg, w)
	}

	return nil
}

func getService(w writer) error {
	return w(true, controlifx.StateServiceType, &implifx.StateServiceLanMessage{
		Service: controlifx.UdpService,
		Port:    uint32(bulb.port),
	})
}

func getHostInfo(w writer) error {
	return w(true, controlifx.StateHostInfoType, &implifx.StateHostInfoLanMessage{})
}

func getHostFirmware(w writer) error {
	return w(true, controlifx.StateHostFirmwareType, &implifx.StateHostFirmwareLanMessage{
		Build:   uint64(bulb.hostFirmware.build),
		Version: bulb.hostFirmware.version,
	})
}

func getWifiInfo(w writer) error {
	return w(true, controlifx.StateWifiInfoType, &implifx.StateWifiInfoLanMessage{
		Signal: bulb.wifiInfo.signal,
		Tx:     bulb.wifiInfo.tx,
		Rx:     bulb.wifiInfo.rx,
	})
}

func getWifiFirmware(w writer) error {
	return w(true, controlifx.StateWifiFirmwareType, &implifx.StateWifiFirmwareLanMessage{
		Build:   uint64(bulb.wifiFirmware.build),
		Version: bulb.wifiFirmware.version,
	})
}

func getPower(w writer) error {
	return w(true, controlifx.StatePowerType, &implifx.StatePowerLanMessage{
		Level: bulb.powerLevel,
	})
}

func setPower(msg implifx.ReceivableLanMessage, w writer) error {
	responsePayload := &implifx.StatePowerLanMessage{
		Level: bulb.powerLevel,
	}
	bulb.powerLevel = msg.Payload.(*implifx.SetPowerLanMessage).Level

	actionCh <- PowerAction(0)

	return w(false, controlifx.StatePowerType, responsePayload)
}

func getLabel(w writer) error {
	return w(true, controlifx.StateLabelType, &implifx.StateLabelLanMessage{
		Label: bulb.label,
	})
}

func setLabel(msg implifx.ReceivableLanMessage, w writer) error {
	bulb.label = msg.Payload.(*implifx.SetLabelLanMessage).Label

	return w(false, controlifx.StateLabelType, &implifx.StateLabelLanMessage{
		Label: bulb.label,
	})
}

func getVersion(w writer) error {
	return w(true, controlifx.StateVersionType, &implifx.StateVersionLanMessage{
		Vendor:  bulb.version.vendor,
		Product: bulb.version.product,
		Version: bulb.version.version,
	})
}

func getInfo(w writer) error {
	now := time.Now().UnixNano()

	return w(true, controlifx.StateInfoType, &implifx.StateInfoLanMessage{
		Time:     uint64(now),
		Uptime:   uint64(now - bulb.startTime),
		Downtime: 0,
	})
}

func getLocation(w writer) error {
	return w(true, controlifx.StateLocationType, &implifx.StateLocationLanMessage{
		Location:  bulb.location.location,
		Label:     bulb.location.label,
		UpdatedAt: uint64(bulb.location.updatedAt),
	})
}

func getGroup(w writer) error {
	return w(true, controlifx.StateGroupType, &implifx.StateGroupLanMessage{
		Group:     bulb.group.group,
		Label:     bulb.group.label,
		UpdatedAt: uint64(bulb.group.updatedAt),
	})
}

func getOwner(w writer) error {
	return w(true, controlifx.StateOwnerType, &implifx.StateOwnerLanMessage{
		Owner:     bulb.owner.owner,
		Label:     bulb.owner.label,
		UpdatedAt: uint64(bulb.owner.updatedAt),
	})
}

func setOwner(msg implifx.ReceivableLanMessage, w writer) error {
	payload := msg.Payload.(*implifx.SetOwnerLanMessage)
	bulb.owner.owner = payload.Owner
	bulb.owner.label = payload.Label
	bulb.owner.updatedAt = time.Now().UnixNano()

	return w(false, controlifx.StateOwnerType, &implifx.StateOwnerLanMessage{
		Owner:     bulb.owner.owner,
		Label:     bulb.owner.label,
		UpdatedAt: uint64(bulb.owner.updatedAt),
	})
}

func echoRequest(msg implifx.ReceivableLanMessage, w writer) error {
	return w(true, controlifx.EchoResponseType, &implifx.EchoResponseLanMessage{
		Payload: msg.Payload.(*implifx.EchoRequestLanMessage).Payload,
	})
}

func lightGet(w writer) error {
	return w(true, controlifx.LightStateType, &implifx.LightStateLanMessage{
		Color: bulb.state.color,
		Power: bulb.powerLevel,
		Label: bulb.label,
	})
}

func lightSetColor(msg implifx.ReceivableLanMessage, w writer) error {
	responsePayload := &implifx.LightStateLanMessage{
		Color: bulb.state.color,
		Power: bulb.powerLevel,
		Label: bulb.label,
	}
	payload := msg.Payload.(*implifx.LightSetColorLanMessage)
	bulb.state.color = payload.Color

	actionCh <- ColorAction{
		Color:    payload.Color,
		Duration: payload.Duration,
	}

	return w(false, controlifx.LightStateType, responsePayload)
}

func lightGetPower(w writer) error {
	return w(true, controlifx.LightStatePowerType, &implifx.LightStatePowerLanMessage{
		Level: bulb.powerLevel,
	})
}

func lightSetPower(msg implifx.ReceivableLanMessage, w writer) error {
	responsePayload := &implifx.StatePowerLanMessage{
		Level: bulb.powerLevel,
	}
	payload := msg.Payload.(*implifx.LightSetPowerLanMessage)
	bulb.powerLevel = payload.Level

	actionCh <- PowerAction(payload.Duration)

	return w(false, controlifx.LightStatePowerType, responsePayload)
}
