package main

import (
	"image"
	"io"
)

type FrameProvider interface {
	io.Closer
	Frame() (*image.RGBA, error)
}

type FrameProviderFactory interface {
	NewFrameProvider() (FrameProvider, error)
}
