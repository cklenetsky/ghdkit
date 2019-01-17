package main

// typedef unsigned char Uint8;
// void SoundCallback(void *userdata, Uint8 * stream, int len);
import "C"
import (
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"

	term "github.com/nsf/termbox-go"
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

var (
	soundPlayer     map[soundType]soundPlayerInfo
	soundPlayerLock sync.RWMutex

	layouts       []map[int]soundType // an array of layouts that map pad/pedal hits to sounds
	currentLayout = DPAD_UP
)

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
		{"snare1.wav", SNARE},
		{"crashCymbal.wav", CRASH_CYMBAL},
		{"hiroomtm.wav", TOM},
		{"Floor-Tom-3.wav", FLOOR_TOM},
		{"Deep-Kick.wav", BASS_PEDAL},
		{"Closed-Hi-Hat-4.wav", HIGH_HAT_CLOSED_HIT},
		{"Ride-Cymbal-2.wav", RIDE_CYMBAL},
	}

	soundPlayerLock.Lock()
	defer soundPlayerLock.Unlock()

	for _, entry := range list {
		if info, err = loadSound(entry.filename, entry.sound); err != nil {
			fmt.Println(err)
		} else {
			soundPlayer[entry.sound] = *info
		}
	}

	layouts = make([]map[int]soundType, 0xff)

	// default layout
	layouts[DPAD_UP] = map[int]soundType{
		RED:       SNARE,
		BLUE:      TOM,
		GREEN:     FLOOR_TOM,
		YELLOW:    HIGH_HAT_CLOSED_HIT,
		ORANGE:    CRASH_CYMBAL,
		KICKPEDAL: BASS_PEDAL,
	}
	// orange ride cymbal
	layouts[DPAD_RIGHT] = map[int]soundType{
		RED:       SNARE,
		BLUE:      TOM,
		GREEN:     FLOOR_TOM,
		YELLOW:    HIGH_HAT_CLOSED_HIT,
		ORANGE:    RIDE_CYMBAL,
		KICKPEDAL: BASS_PEDAL,
	}
	// orange ride cymbal, yellow crash cymbal
	layouts[DPAD_DOWN] = map[int]soundType{
		RED:       SNARE,
		BLUE:      TOM,
		GREEN:     FLOOR_TOM,
		YELLOW:    CRASH_CYMBAL,
		ORANGE:    RIDE_CYMBAL,
		KICKPEDAL: BASS_PEDAL,
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
			immediatePlaySound(sound)
		case bye := <-quit:
			if bye {
				return
			}
		}
	}
}

func main() {
	var sdlVer sdl.Version

	err := sdl.InitSubSystem(sdl.INIT_GAMECONTROLLER | sdl.INIT_JOYSTICK | sdl.INIT_AUDIO)
	if err != nil {
		log.Fatal(err)
	}
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

	quitChan := make(chan bool)
	soundChan := make(chan soundType)

	err = term.Init()
	if err == nil {
		defer term.Close()
		fmt.Println("Key input mode")
		fmt.Println("[1]     [3]")
		fmt.Println("[q] [w] [e]")
		fmt.Println("[  space  ]")
	}

	sdl.JoystickEventState(sdl.ENABLE)
	var j *sdl.Joystick
	if num >= 0 {
		j = sdl.JoystickOpen(num)
		if j == nil {
			fmt.Printf("Couldn't open Joystick %d\n", num)
		}
	}
	if j != nil {
		fmt.Printf("Opened Joystick %d\n", num)
		fmt.Printf("Name: %s\n", j.Name())
		fmt.Printf("Number of Axes: %d\n", j.NumAxes())
		fmt.Printf("Number of Buttons: %d\n", j.NumButtons())
		fmt.Printf("Number of Balls: %d\n", j.NumBalls())
		fmt.Printf("Number of Hats: %d\n", j.NumHats())
		defer j.Close()
	}

	done := false

	initSounds()
	defer closeSounds()

	go playSound(soundChan, quitChan)

	fmt.Println(j)
	for !done {
		if j != nil {
			for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
				switch event.(type) {
				case *sdl.JoyButtonEvent:
					jevent := event.(*sdl.JoyButtonEvent)
					if jevent.State == sdl.PRESSED {
						switch jevent.Button {
						case RED:
							fallthrough
						case BLUE:
							fallthrough
						case GREEN:
							fallthrough
						case YELLOW:
							fallthrough
						case ORANGE:
							fallthrough
						case KICKPEDAL:
							soundChan <- layouts[currentLayout][int(jevent.Button)]
						}
					} else {
						switch jevent.Button {
						case BUTTON_PS:
							quitChan <- true
							done = true
						}
					}
				case *sdl.JoyHatEvent:
					jevent := event.(*sdl.JoyHatEvent)
					fmt.Printf("Hat %d, value %d\n", jevent.Hat, jevent.Value)
					switch jevent.Value {
					case DPAD_UP:
						fallthrough
					case DPAD_RIGHT:
						fallthrough
					case DPAD_DOWN:
						fallthrough
					case DPAD_LEFT:
						if _, found := layouts[jevent.Value][RED]; found {
							currentLayout = int(jevent.Value)
						}
					}
				case *sdl.JoyAxisEvent:
					jevent := event.(*sdl.JoyAxisEvent)
					fmt.Printf("Axis %d, value %d\n", jevent.Axis, jevent.Value)
				}
			}
		} else {
			switch ev := term.PollEvent(); ev.Type {
			case term.EventKey:
				switch ev.Key {
				case term.KeyEsc:
					quitChan <- true
					done = true
				case term.KeyArrowUp:
					if _, found := layouts[DPAD_UP][RED]; found {
						currentLayout = int(DPAD_UP)
					}
				case term.KeyArrowRight:
					if _, found := layouts[DPAD_RIGHT][RED]; found {
						currentLayout = int(DPAD_RIGHT)
					}
				case term.KeyArrowDown:
					if _, found := layouts[DPAD_DOWN][RED]; found {
						currentLayout = int(DPAD_DOWN)
					}
				case term.KeyArrowLeft:
					if _, found := layouts[DPAD_LEFT][RED]; found {
						currentLayout = int(DPAD_LEFT)
					}
				case term.KeySpace:
					soundChan <- layouts[currentLayout][KICKPEDAL]
				default:
					switch ev.Ch {
					case 'q':
						soundChan <- layouts[currentLayout][RED]
					case 'w':
						soundChan <- layouts[currentLayout][BLUE]
					case 'e':
						soundChan <- layouts[currentLayout][GREEN]
					case '1':
						soundChan <- layouts[currentLayout][YELLOW]
					case '3':
						soundChan <- layouts[currentLayout][ORANGE]
					}
				}
			}
		}
	}
	fmt.Println("Done.")
}
