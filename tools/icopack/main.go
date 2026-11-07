// icopack arma un .ico de Windows a partir de varios PNG (uno por tamaño).
// Solo usa la librería estándar de Go.
//
// Uso (desde la raíz del repo):
//
//	go run ./tools/icopack -o build/windows/icon.ico icon-16.png icon-24.png ... icon-256.png
//
// Los PNG se exportan desde build/kiltro.svg (ver build/README.md). El de 256
// se guarda tal cual (PNG dentro del .ico, lo normal desde Windows Vista); los
// más chicos se convierten a BMP de 32 bits, que es lo que mejor entiende la
// bandeja del sistema.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"sort"
)

type entry struct {
	size int
	data []byte
}

func main() {
	out := flag.String("o", "icon.ico", "archivo .ico de salida")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "uso: icopack -o salida.ico a.png b.png ...")
		os.Exit(2)
	}

	var entries []entry
	for _, path := range flag.Args() {
		raw, err := os.ReadFile(path)
		if err != nil {
			fail(err)
		}
		img, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			fail(fmt.Errorf("%s: %w", path, err))
		}
		b := img.Bounds()
		if b.Dx() != b.Dy() || b.Dx() > 256 {
			fail(fmt.Errorf("%s: tiene que ser cuadrado y de 256 px o menos (es %dx%d)", path, b.Dx(), b.Dy()))
		}
		e := entry{size: b.Dx()}
		if e.size == 256 {
			e.data = raw
		} else {
			e.data = toDIB(img)
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].size < entries[j].size })

	var buf bytes.Buffer
	// ICONDIR: reservado (0), tipo (1 = ícono), cantidad de imágenes.
	binary.Write(&buf, binary.LittleEndian, []uint16{0, 1, uint16(len(entries))})
	offset := 6 + 16*len(entries)
	for _, e := range entries {
		dim := byte(e.size) // 256 se escribe como 0
		buf.Write([]byte{dim, dim, 0, 0})
		binary.Write(&buf, binary.LittleEndian, []uint16{1, 32}) // planos, bits por píxel
		binary.Write(&buf, binary.LittleEndian, []uint32{uint32(len(e.data)), uint32(offset)})
		offset += len(e.data)
	}
	for _, e := range entries {
		buf.Write(e.data)
	}
	if err := os.WriteFile(*out, buf.Bytes(), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("%s: %d tamaños, %d bytes\n", *out, len(entries), buf.Len())
}

// toDIB convierte la imagen a BMP de 32 bits (BGRA) como va dentro de un .ico:
// cabecera BITMAPINFOHEADER con el doble de alto (imagen + máscara), filas de
// abajo hacia arriba, y al final la máscara AND (toda en 0: la transparencia
// la da el canal alfa).
func toDIB(img image.Image) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, struct {
		Size, Width, Height int32
		Planes, BitCount    uint16
		Compression, Image  uint32
		XPels, YPels        int32
		ClrUsed, ClrImp     uint32
	}{40, int32(w), int32(h * 2), 1, 32, 0, uint32(w * h * 4), 0, 0, 0, 0})

	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			// RGBA() viene premultiplicado por alfa; el .ico espera colores sin premultiplicar.
			if a > 0 {
				r, g, bl = r*0xffff/a, g*0xffff/a, bl*0xffff/a
			}
			buf.Write([]byte{byte(bl >> 8), byte(g >> 8), byte(r >> 8), byte(a >> 8)})
		}
	}
	rowBytes := ((w + 31) / 32) * 4 // cada fila de la máscara se rellena a 4 bytes
	buf.Write(make([]byte, rowBytes*h))
	return buf.Bytes()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
