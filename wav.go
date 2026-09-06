package main

import (
	"encoding/binary"
	"io"
	"os"
)

// WriteWavFile salva amostras float32 como um arquivo WAV padrão 16-bit PCM mono
func WriteWavFile(filename string, samples []float32, sampleRate int) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	numSamples := len(samples)
	numChannels := 1
	bitsPerSample := 16
	bytesPerSample := bitsPerSample / 8
	blockAlign := numChannels * bytesPerSample
	byteRate := sampleRate * blockAlign
	dataSize := numSamples * blockAlign
	chunkSize := 36 + dataSize

	// Cabeçalho RIFF
	if _, err := file.WriteString("RIFF"); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(chunkSize)); err != nil {
		return err
	}
	if _, err := file.WriteString("WAVE"); err != nil {
		return err
	}

	// Subchunk "fmt "
	if _, err := file.WriteString("fmt "); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(16)); err != nil { // Subchunk1Size para PCM = 16
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(1)); err != nil { // AudioFormat = 1 (PCM)
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(numChannels)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(byteRate)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(blockAlign)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(bitsPerSample)); err != nil {
		return err
	}

	// Subchunk "data"
	if _, err := file.WriteString("data"); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(dataSize)); err != nil {
		return err
	}

	// Converte float32 (-1.0 a 1.0) para int16
	buf := make([]byte, 2)
	for _, sample := range samples {
		// Clamp entre -1.0 e 1.0
		if sample > 1.0 {
			sample = 1.0
		} else if sample < -1.0 {
			sample = -1.0
		}
		val := int16(sample * 32767.0)
		binary.LittleEndian.PutUint16(buf, uint16(val))
		if _, err := file.Write(buf); err != nil {
			return err
		}
	}

	return nil
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

