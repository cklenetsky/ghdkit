package main

// typedef unsigned char Uint8;
// void SoundCallback(void *userdata, Uint8 * stream, int len);
import "C"
import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"sync"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
)

type soundType int

const (
	BUTTON_SQUARE   = 0
	BUTTON_X        = 1
	BUTTON_CIRCLE   = 2
	BUTTON_TRIANGLE = 3
	BUTTON_SELECT   = 8
	BUTTON_START    = 9
	BUTTON_PS       = 12

	KICKPEDAL = 4
	RED       = BUTTON_CIRCLE
	BLUE      = BUTTON_SQUARE
	GREEN     = BUTTON_X
	YELLOW    = BUTTON_TRIANGLE
	ORANGE    = 5

	DPAD_UP    = 0x01
	DPAD_RIGHT = 0x02
	DPAD_DOWN  = 0x04
	DPAD_LEFT  = 0x08

	BASS_PEDAL soundType = iota
	SNARE
	TOM
	FLOOR_TOM
	HIGH_HAT_CLOSED_HIT
	HIGH_HAT_OPEN_HIT
	CRASH_CYMBAL
	RIDE_CYMBAL
)

type soundPlayerInfo struct {
	data    []byte            // the bytes to output to the player
	index   uint              // the current position in the data
	audioID sdl.AudioDeviceID // the SDL audio device id to play this sound
}

var (
	soundPlayer     map[soundType]soundPlayerInfo
	soundPlayerLock sync.RWMutex
)

//export SoundCallback
func SoundCallback(userdata unsafe.Pointer, stream *C.Uint8, length C.int) {
	n := int(length)
	hdr := reflect.SliceHeader{Data: uintptr(unsafe.Pointer(stream)), Len: n, Cap: n}
	buf := *(*[]C.Uint8)(unsafe.Pointer(&hdr))

	sound := soundType(uintptr(userdata))
	soundPlayerLock.RLock()
	info, found := soundPlayer[sound]
	soundPlayerLock.RUnlock()
	for i := 0; i < n; i += 2 {
		if !found || info.index+1 > uint(len(info.data)) {
			buf[i] = 0
			buf[i+1] = 0
		} else {
			buf[i] = C.Uint8(info.data[info.index])
			buf[i+1] = C.Uint8(info.data[info.index+1])
		}
		info.index += 2
		soundPlayerLock.Lock()
		soundPlayer[sound] = info
		soundPlayerLock.Unlock()
	}
}

func loadSoundDevice(filename string, sound soundType) (*soundPlayerInfo, error) {

	buffer, audioSpec := sdl.LoadWAV(filename)
	fmt.Printf("%d bytes loaded from %s\n", len(buffer), filename)
	audioSpec.UserData = unsafe.Pointer(uintptr(sound))
	audioSpec.Callback = sdl.AudioCallback(C.SoundCallback)
	deviceID, err := sdl.OpenAudioDevice("", false, audioSpec, nil, 0)
	if err != nil {
		return nil, err
	}
	fmt.Printf("DeviceID=%d\n", deviceID)
	return &soundPlayerInfo{
		data:    buffer,
		index:   0,
		audioID: deviceID,
	}, nil
}

func initSounds() {
	var (
		info *soundPlayerInfo
		err  error
	)

	soundPlayer = make(map[soundType]soundPlayerInfo)

	list := []struct {
		filename string
		sound    soundType
	}{
		{"lochrome.wav", SNARE},
		{"crashCymbal.wav", CRASH_CYMBAL},
		{"hiroomtm.wav", TOM},
		{"Floor-Tom-3.wav", FLOOR_TOM},
		{"Deep-Kick.wav", BASS_PEDAL},
		{"Closed-Hi-Hat-4.wav", HIGH_HAT_CLOSED_HIT},
	}

	soundPlayerLock.Lock()
	defer soundPlayerLock.Unlock()

	for _, entry := range list {
		if info, err = loadSoundDevice(entry.filename, entry.sound); err != nil {
			fmt.Println(err)
		} else {
			soundPlayer[entry.sound] = *info
		}
	}
}

func closeSounds() {
	for _, info := range soundPlayer {
		sdl.CloseAudioDevice(info.audioID)
		sdl.FreeWAV(info.data)
	}
}

func playSound(what chan soundType, quit chan bool) {
	nameMap := map[soundType]string{
		BASS_PEDAL:          "bass pedal",
		SNARE:               "snare hit",
		TOM:                 "tom tom",
		FLOOR_TOM:           "floor tom",
		HIGH_HAT_CLOSED_HIT: "closed high hat",
		HIGH_HAT_OPEN_HIT:   "open high hat",
		CRASH_CYMBAL:        "crash cymbal",
		RIDE_CYMBAL:         "ride cymbal",
	}

	for {
		select {
		case sound := <-what:
			fmt.Println(nameMap[sound])
			soundPlayerLock.RLock()
			s, found := soundPlayer[sound]
			soundPlayerLock.RUnlock()
			if found {
				sdl.PauseAudioDevice(s.audioID, true)
				s.index = 0
				soundPlayerLock.Lock()
				soundPlayer[sound] = s
				soundPlayerLock.Unlock()
				sdl.PauseAudioDevice(s.audioID, false)
			}
		case bye := <-quit:
			if bye {
				return
			}
		}
	}
}

func main() {
	var sdlVer sdl.Version

	sdl.InitSubSystem(sdl.INIT_GAMECONTROLLER | sdl.INIT_JOYSTICK | sdl.INIT_AUDIO)
	sdl.VERSION(&sdlVer)
	fmt.Printf("Using SDL %d.%d.%d\n", sdlVer.Major, sdlVer.Minor, sdlVer.Patch)
	audioDeviceCount := sdl.GetNumAudioDevices(false)
	for i := 0; i < audioDeviceCount; i++ {
		fmt.Printf("Audio device %d: %s\n", i, sdl.GetAudioDeviceName(i, false))
	}

	if len(os.Args) < 2 {
		js := sdl.NumJoysticks()
		fmt.Printf("Found %d joystick(s)\n", js)
		for i := 0; i < js; i++ {
			if !sdl.IsGameController(i) {
				j := sdl.JoystickOpen(i)
				fmt.Printf("Joystick %d: %s", i, j.Name())
				continue
			}
			gp := sdl.GameControllerOpen(int(i))
			if gp != nil {
				fmt.Printf("Gamepad %d: %s\n", i, gp.Name())
				gp.Close()
			}
		}
		return
	}

	tmp, err := strconv.ParseInt(os.Args[1], 0, 64)
	if err != nil {
		fmt.Println(err)
		return
	}
	num := int(tmp)

	// Open
	if sdl.IsGameController(num) {
		gp := sdl.GameControllerOpen(num)
		if gp != nil {
			fmt.Printf("Opened Gamepad %d\n", num)
			fmt.Printf("Name: %s\n", gp.Name())
			fmt.Printf("Gamepad mapping: %s\n", gp.Mapping())

			// Close if opened
			gp.Close()
		} else {
			fmt.Printf("Couldn't open Gamepad %d\n", num)
		}
	} else {

		quitChan := make(chan bool)
		soundChan := make(chan soundType)

		sdl.JoystickEventState(sdl.ENABLE)
		j := sdl.JoystickOpen(num)
		if j != nil {
			fmt.Printf("Opened Joystick %d\n", num)
			fmt.Printf("Name: %s\n", j.Name())
			fmt.Printf("Number of Axes: %d\n", j.NumAxes())
			fmt.Printf("Number of Buttons: %d\n", j.NumButtons())
			fmt.Printf("Number of Balls: %d\n", j.NumBalls())
			fmt.Printf("Number of Hats: %d\n", j.NumHats())

			done := false

			initSounds()
			defer closeSounds()

			go playSound(soundChan, quitChan)

			for !done {
				for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
					switch event.(type) {
					case *sdl.JoyButtonEvent:
						jevent := event.(*sdl.JoyButtonEvent)
						if jevent.State == sdl.PRESSED {
							switch jevent.Button {
							case RED:
								soundChan <- SNARE
							case BLUE:
								soundChan <- TOM
							case GREEN:
								soundChan <- FLOOR_TOM
							case YELLOW:
								soundChan <- HIGH_HAT_CLOSED_HIT
							case ORANGE:
								soundChan <- CRASH_CYMBAL //RIDE_CYMBAL
							case KICKPEDAL:
								soundChan <- BASS_PEDAL
							}
						} else {
							if jevent.Button == BUTTON_PS {
								quitChan <- true
								done = true
							}
						}
					case *sdl.JoyAxisEvent:
						jevent := event.(*sdl.JoyAxisEvent)
						fmt.Printf("Axis %d, value %d\n", jevent.Axis, jevent.Value)
					case *sdl.JoyHatEvent:
						jevent := event.(*sdl.JoyHatEvent)
						fmt.Printf("Hat %d, value %d\n", jevent.Hat, jevent.Value)
					}
				}
			}

			fmt.Println("Done.")
			// Close if opened
			j.Close()
		} else {
			fmt.Printf("Couldn't open Joystick %d\n", num)
		}
	}
}
