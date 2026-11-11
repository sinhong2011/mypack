// Command logocolor prints the dominant saturated colours of an image
// (used to derive mypack's primary colour from the app logo).
package main

import (
	"fmt"
	"image"
	_ "image/png"
	"os"
	"sort"
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	img, _, err := image.Decode(f)
	if err != nil {
		panic(err)
	}
	type bin struct{ n, r, g, b float64 }
	bins := map[int]*bin{}
	var total float64
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			r, g, bl, a := img.At(x, y).RGBA()
			if a < 0xf000 {
				continue
			}
			R, G, B := float64(r>>8), float64(g>>8), float64(bl>>8)
			mx, mn := max(R, G, B), min(R, G, B)
			if mx-mn < 90 { // skip whites/greys (the box and window)
				continue
			}
			k := int(R)>>4<<8 | int(G)>>4<<4 | int(B)>>4
			if bins[k] == nil {
				bins[k] = &bin{}
			}
			bn := bins[k]
			bn.n++
			bn.r += R
			bn.g += G
			bn.b += B
			total++
		}
	}
	var list []*bin
	for _, v := range bins {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
	var ar, ag, ab float64
	for i, v := range list {
		if i < 5 {
			fmt.Printf("#%02x%02x%02x  %.1f%%\n", int(v.r/v.n), int(v.g/v.n), int(v.b/v.n), 100*v.n/total)
		}
		ar += v.r
		ag += v.g
		ab += v.b
	}
	fmt.Printf("mean saturated: #%02x%02x%02x\n", int(ar/total), int(ag/total), int(ab/total))
}
