// Copyright 2018 David H. Wells, Jr. All rights reserved.
// Use of this source code is governed by the GNU General
// Public License that can be found in the LICENSE file.

// morse.go converts text from Stdin to an audio file cw.wav
//  of Morse Code in signed 16-bit little endian format.
//  on a linux system: lame cw.wav newFileName.mp3
//  will write an mp3 file

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
)

const rate = 11025 // samples per second
var dot int        // samples per dot, sets the character speed, 750 is ~ 14 wpm
var farns float32  // farnsworth dots of quiet added to intervals between chars and (*2) between words
var bfo int        // audible freqency in Hz, default 660

var morse_code = map[byte]string{
	'a': ".-", 'b': "-...", 'c': "-.-.",
	'd': "-..", 'e': ".", 'f': "..-.",
	'g': "--.", 'h': "....", 'i': "..",
	'j': ".---", 'k': "-.-", 'l': ".-..",
	'm': "--", 'n': "-.", 'o': "---",
	'p': ".--.", 'q': "--.-", 'r': ".-.",
	's': "...", 't': "-", 'u': "..-",
	'v': "...-", 'w': ".--", 'x': "-..-",
	'y': "-.--", 'z': "--..",
	'1': ".----", '2': "..---", '3': "...--",
	'4': "....-", '5': ".....", '6': "-....",
	'7': "--...", '8': "---..", '9': "----.",
	'0': "-----",
	'.': ".-.-.-", ',': "--..--", '?': "..--..",
	'"': ".-..-.", '/': "-..-.", ':': "---...",
	'\'': ".----.", // according to Consoli
	'-':  "-....-",
	'=':  "-...-",  // break/separator     prosign BT (double-dash)
	'+':  ".-.-.",  // stop/end of message prosign AR
	'&':  ".-...",  // wait                prosign AS
	'$':  "...-.-", // end of transmission prosign SK
	'@':  ".--.-.",
	' ':  " ", // space (between words) is 4 quiet dots worth, not counting the "std" quiet after char
	'\n': " ", // end of line equivalent to a space
}

// As explained by Consoli-------------------------------------------------
// A dot is the base unit of measure.
// A dit is a sound of one dot followed by a silence of one dot
// A daah, the sound of a dash, is 3 dots long followed by a silence of one
// dot.
// The space between letters is 3 dots long. (the dot ending the preceding
// dit or daah plus 2 dots)
// The space between words is 7 dots long (3 of the preceding letter space
// plus 4 dots), note don't expect to have multiple consecutive spaces
//------------------------------------------------------------------------

var tone1, tone3, quietSp []byte

func initClips() {
	tone1 = tone(1)
	tone3 = tone(3)
	quietSp = quiet(2 + farns)
}

func play(b []byte) (wav []byte) {
	for _, ch := range b {
		seq := morse_code[ch]
		for _, s := range seq {
			switch s {
			case '.':
				wav = append(wav, tone1...)
			case '-':
				wav = append(wav, tone3...)
			case ' ':
				wav = append(wav, quietSp...) // between words, std 4, f=2 gives 8
			default:
				fmt.Println("problem sounding this character", s)
				os.Exit(6)
			}
		}
		// note this space is also added after ' '
		wav = append(wav, quietSp...) // between ch, std 2, f=2 gives 4
	}
	wav = wav[:]
	return
}

func main() {
	var wpm, eff int
	var help bool
	flag.IntVar(&eff, "f", 9, "farnsworth rate, effective wpm")
	flag.IntVar(&wpm, "w", 22, "character rate, wpm, words per minute")
	flag.IntVar(&bfo, "t", 660, "Hz for BFO")
	flag.BoolVar(&help, "h", false, "brief help")
	flag.Parse()
	helpmsg := "Morse code audio generator, generates cw.wav from stdin text\n" +
		"  see go code for mapping of prosigns to ascii characters\n" +
		"  example:\n" +
		"   go run morse.go <textFile\n" +
		"  switches:\n" +
		"   -f  farnsworth rate is the effective words per minute (wpm) sent, default 9\n" +
		"   -w  character rate, the rate at which individual characters are sent, default 22\n" +
		"   -t  tone frequency in Hz, default 660\n" +
		"   -h  print this help message\n"
	if help {
		fmt.Println(helpmsg)
		os.Exit(0)
	}
	farns = 50 * float32(wpm-eff) / float32(eff) / 7
	fmt.Println("farnsworth factor", farns)
	fmt.Println("digital samples per dot", 13230/wpm)
	fmt.Println("dot duration in milliseconds", 1200/wpm)
	dot = int(13230 / wpm)
	wav := make([]byte, 0)
	const LEN = 100
	bailout := false
	buf := make([]byte, LEN)
	initClips()

	var nr int
	var err error
	for {
		switch nr, err = os.Stdin.Read(buf); true {
		case nr > 0:
			wav = append(wav, play(bytes.ToLower(buf[0:nr]))...)
		case nr == 0:
			bailout = true
		case err != nil:
			if err != io.EOF {
				fmt.Println("problem reading standard input", err)
				os.Exit(3)
			}
		}
		if bailout || err == io.EOF {
			break
		}
	}
	makeWav(wav, "cw")
}

// tonedur is the number of "dots" of perfect quiet (two bytes per sample)
// allow quiet to be called with a float to accomodate Farnsworth factors that aren't integer
func quiet(tonedur float32) []byte {
	tone := make([]byte, 2*int(tonedur*float32(dot)))
	return tone
}

func tone(tonedur int) []byte {
	// sine wave
	hz := float64(bfo)
	// rise & fall time in seconds
	rampTau := 3.0E-3
	delay := 1.5 * rampTau
	tone := make([]byte, 2*(tonedur+1)*dot)
	toneSecs := float64(tonedur)*float64(dot)/float64(rate) - rampTau
	for tics := 0; tics < 2*(tonedur+1)*dot; tics += 2 {
		seconds := float64(tics) / (2.0 * float64(rate))
		cycle := math.Pi * 2.0 * hz * seconds
		cycle = math.Sin(cycle) // -1..1
		ramp := (math.Erf((seconds-delay)/rampTau) -
			math.Erf((seconds-delay-toneSecs)/rampTau)) / 2.0
		tt := int16(32766 * cycle * ramp)
		tone[tics] = byte(tt)
		tone[tics+1] = byte(tt >> 8)
	}
	return tone
}

func bytes4(x uint32) []byte {
	buf := make([]byte, 4)
	buf[0] = byte(x >> 0)
	buf[1] = byte(x >> 8)
	buf[2] = byte(x >> 16)
	buf[3] = byte(x >> 24)
	return buf
}

func makeWav(tone []byte, fn string) {
	blank := []byte("\x80")
	export := append(tone, blank...)
	samples := uint32(len(export))
	dz := "\x00\x00"
	sz := "\x00"
	s1 := "\x01"
	samp := "\x11\x2b" + dz  // 11025 sample rate (32-bit little endian)
	samp2 := "\x22\x56" + dz // 2 * 11025 sample rate
	riff := append(append([]byte("RIFF"), bytes4(samples+36)...), []byte("WAVE")...)
	//                                   16=>pcm            1=>pcm
	format := []byte("fmt\x20" + ("\x10" + sz + sz + sz) + (s1 + sz) +
		// 1 chan    samp   r*ch*b/8     align           bits per samp
		(s1 + sz) + (samp) + (samp2) + ("\x02" + sz) + ("\x10" + sz))
	data := append([]byte("data"), bytes4(samples)...)
	doWrite(fn+".wav", append(append(append(riff, format...), data...), export...))
}

func doWrite(fn string, b []byte) (err error) {
	f, err := os.Create(fn)
	if err != nil {
		return
	}
	defer f.Close()
	_, err = f.Write(b)
	if err != nil {
		return
	}
	return
}
