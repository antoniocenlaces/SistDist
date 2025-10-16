package main

import (
	"fmt"
)

type Vertex struct {
	X float64
	Y float64
}

func (v *Vertex) Scale(f float64) {
	v.X = v.X * f
	v.Y = v.Y * f
}

func main() {
	v := Vertex{3, 2}
	v.Scale(10)
	fmt.Println(v)
}
