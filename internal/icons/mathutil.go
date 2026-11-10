package icons

import "math"

func hypot(a, b float64) float64 { return math.Hypot(a, b) }
func absf(a float64) float64     { return math.Abs(a) }
func clamp01(a float64) float64  { return math.Max(0, math.Min(1, a)) }
