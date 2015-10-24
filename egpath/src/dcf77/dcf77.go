// Package dcf77 decodes DCF77 time signal.
package dcf77

import (
	"errors"
	"fmt"
	"time"
)

type Error string

func (e *Error) Error() string {
	return string(*e)
}

var (
	ErrInit   = errors.New("initializing")
	ErrTiming = errors.New("timing error")
	ErrBits   = errors.New("error in data bits")
)

type Time struct {
	Year   byte
	Month  byte
	Mday   byte
	Wday   byte
	Hour   byte
	Min    byte
	Sec    byte
	Summer bool
}

func decodeBCD(b byte) byte {
	return (b>>4)*10 + b&0x0f
}

func (t *Time) decodeBCD() {
	t.Year = decodeBCD(t.Year)
	t.Month = decodeBCD(t.Month)
	t.Mday = decodeBCD(t.Mday)
	t.Hour = decodeBCD(t.Hour)
	t.Min = decodeBCD(t.Min)
}

func (t Time) Format(f fmt.State, _ rune) {
	zone := " CET"
	if t.Summer {
		zone = " CEST"
	}
	fmt.Fprintf(
		f,
		"%02d-%02d-%02d %02d:%02d:%02d %s",
		t.Year, t.Month, t.Mday, t.Hour, t.Min, t.Sec, zone,
	)
}

// Pulse represents information about received pulse.
type Pulse struct {
	Time            // Received time.
	Stamp time.Time // Local time of received pulse (rising edge).
	Err   error
}

type Decoder struct {
	pulse Pulse
	next  Time
	last  time.Time
	sec   int
	ones  int
	c     chan Pulse
}

func (d *Decoder) error(err error) (de bool) {
	if de = d.pulse.Err != err; de {
		d.pulse.Err = err
	}
	d.sec = -1
	d.ones = 0
	return
}

// NewDecoder returns pointer to new ready to use DCF77 signal decoder.
func NewDecoder() *Decoder {
	d := new(Decoder)
	d.c = make(chan Pulse, 1)
	d.error(ErrTiming)
	return d
}

func checkRising(dt64 time.Duration) int {
	if dt64 > 2050e6 {
		return -1
	}
	dt := uint(dt64)
	switch {
	case dt > 1950e6:
		return 1
	case dt > 1050e6:
		return -1
	case dt > 950e6:
		return 0
	}
	return -1
}

func (d *Decoder) risingEdge(dt time.Duration) (send bool) {
	switch checkRising(dt) {
	case 0: // Ordinary pulse.
		if d.sec >= 0 {
			d.sec++
			if d.pulse.Err == nil {
				d.pulse.Sec = byte(d.sec)
			}
			send = true
		}
	case 1: // Sync pulse.
		if d.sec >= 0 {
			d.pulse.Time = d.next
			d.pulse.Err = nil
		} else {
			d.pulse.Err = ErrInit
		}
		d.sec = 0
		d.next = Time{}
		send = true
	default: // Error
		send = d.error(ErrTiming)
	}
	return
}

func checkFalling(dt64 time.Duration) int {
	if dt64 > 250e6 {
		return -1
	}
	dt := uint(dt64)
	switch {
	case dt > 140e6:
		return 1
	case dt > 130e6:
		return -1
	case dt > 40e6:
		return 0
	}
	return -1
}

func (d *Decoder) fallingEdge(dt time.Duration) (send bool) {
	bit := checkFalling(dt)
	if bit < 0 {
		send = d.error(ErrTiming)
		return
	}
	switch {
	case d.sec == 0:
		if bit != 0 {
			send = d.error(ErrBits)
		}
	case d.sec <= 16:
		// Don't decode.
	case d.sec == 17:
		d.next.Summer = (bit == 1)
	case d.sec == 18:
		if d.next.Summer == (bit == 1) {
			send = d.error(ErrBits)
		}
	case d.sec == 19:
		// Leap second announcement.
	case d.sec == 20:
		if bit == 0 {
			send = d.error(ErrBits)
		}
	case d.sec <= 27:
		if bit != 0 {
			d.ones++
			d.next.Min += 1 << uint(d.sec-21)
		}
	case d.sec == 28:
		if bit != 0 {
			d.ones++
		}
		if d.ones&1 != 0 {
			send = d.error(ErrBits)
		}
	case d.sec <= 34:
		if bit != 0 {
			d.ones++
			d.next.Hour += 1 << uint(d.sec-29)
		}
	case d.sec == 35:
		if bit != 0 {
			d.ones++
		}
		if d.ones&1 != 0 {
			send = d.error(ErrBits)
		}
	case d.sec <= 41:
		if bit != 0 {
			d.ones++
			d.next.Mday += 1 << uint(d.sec-36)
		}
	case d.sec <= 44:
		if bit != 0 {
			d.ones++
			d.next.Wday += 1 << uint(d.sec-42)
		}
	case d.sec <= 49:
		if bit != 0 {
			d.ones++
			d.next.Month += 1 << uint(d.sec-45)
		}
	case d.sec <= 57:
		if bit != 0 {
			d.ones++
			d.next.Year += 1 << uint(d.sec-50)
		}
	case d.sec == 58:
		if bit != 0 {
			d.ones++
		}
		if d.ones&1 != 0 {
			send = d.error(ErrBits)
		}
	}
	return
}

// Edge should be called by interrupt handler trigered by both (rising and
// falling) edges of DCF77 signal pulses.
func (d *Decoder) Edge(t time.Time, rising bool) {
	dt := t.Sub(d.last)
	send := false
	if rising {
		d.last = t
		send = d.risingEdge(dt)
		if d.pulse.Err == nil {
			d.pulse.Stamp = t
		}
	} else {
		send = d.fallingEdge(dt)
	}
	if send {
		select {
		case d.c <- d.pulse:
		default:
		}
	}
}

// Pulse returns next decoded pulse. It can return buffered value, so if called
// with period > 1 second, it should be called twice to obtain most recent
// value.
func (d *Decoder) Pulse() Pulse {
	p := <-d.c
	p.Time.decodeBCD()
	return p
}
