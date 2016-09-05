package main

import "math"

func abs(a int32) int32 {
	if a < 0 {
		return -a
	}

	return a
}

func lerp(durationStart, duration, now int64, vStart, vChange int32) int32 {
	return int32(float32(now-durationStart)/float32(duration)*float32(vChange) + float32(vStart))
}

// Credit to http://www.tannerhelland.com/4435/convert-temperature-rgb-algorithm-code/.
func kToRgb(k float32) (r, g, b float32) {
	k /= 100

	// Red.
	if k <= 66 {
		r = 255
	} else if r = 329.698727446 * float32(math.Pow(float64(k-60), -0.1332047592)); r < 0 {
		r = 0
	} else if r > 255 {
		r = 255
	}

	// Green.
	if k <= 66 {
		if g = 99.4708025861*float32(math.Log(float64(k))) - 161.1195681661; g < 0 {
			g = 0
		} else if g > 255 {
			g = 255
		}
	} else if g = 288.1221695283 * float32(math.Pow(float64(k-60), -0.0755148492)); g < 0 {
		g = 0
	} else if g > 255 {
		g = 255
	}

	// Blue.
	if k >= 66 {
		b = 255
	} else if k <= 19 {
		b = 0
	} else if b = 138.5177312231*float32(math.Log(float64(k-10))) - 305.0447927307; b < 0 {
		b = 0
	} else if b > 255 {
		b = 255
	}

	return r / 255, g / 255, b / 255
}

func hslToRgb(h, s, l float32) (r, g, b float32) {
	if s == 0 {
		r = l
		g = l
		b = l
	} else {
		hueToRgb := func(p, q, t float32) float32 {
			if t < 0 {
				t += 1
			} else if t > 1 {
				t -= 1
			}

			if t < 1/6.0 {
				return p + (q-p)*6*t
			}
			if t < 1/2.0 {
				return q
			}
			if t < 2/3.0 {
				return p + (q-p)*(2/3.0-t)*6
			}
			return p
		}

		var q float32

		if l < 0.5 {
			q = l * (1 + s)
		} else {
			q = l + s - l*s
		}

		p := 2*l - q
		r = hueToRgb(p, q, h+1/3.0)
		g = hueToRgb(p, q, h)
		b = hueToRgb(p, q, h-1/3.0)
	}

	return
}

func durationToNano(d uint32) int64 {
	return int64(d) * 1e6
}
