package main

import (
	"encoding/binary"
)

type SerialisationContext struct {
	buffer    []byte
	bufferLoc int
	isReading bool
	err       error
}

func newSerialisation(buffer []byte, isReading bool) SerialisationContext {
	result := SerialisationContext{
		buffer,
		0,
		isReading,
		nil,
	}
	return result
}

func (ctx *SerialisationContext) assert(condition bool) {
	if (ctx.err == nil) && !condition {
		ctx.err = ErrInvalidData
	}
}

func (ctx *SerialisationContext) serialiseByte(val *byte) {
	ctx.ensureFreeBufferSpace(1)
	if ctx.err != nil {
		return
	}

	if ctx.isReading {
		*val = ctx.buffer[ctx.bufferLoc]
	} else {
		ctx.buffer[ctx.bufferLoc] = *val
	}
	ctx.bufferLoc += 1
}

func (ctx *SerialisationContext) serialiseUint16(val *uint16) {
	ctx.ensureFreeBufferSpace(2)
	if ctx.err != nil {
		return
	}

	slice := ctx.buffer[ctx.bufferLoc : ctx.bufferLoc+2]
	if ctx.isReading {
		*val = binary.LittleEndian.Uint16(slice)
	} else { // writing
		binary.LittleEndian.PutUint16(slice, *val)
	}
	ctx.bufferLoc += 2
}

func (ctx *SerialisationContext) serialiseUint64(val *uint64) {
	ctx.ensureFreeBufferSpace(8)
	if ctx.err != nil {
		return
	}

	slice := ctx.buffer[ctx.bufferLoc : ctx.bufferLoc+8]
	if ctx.isReading {
		*val = binary.LittleEndian.Uint64(slice)
	} else { // writing
		binary.LittleEndian.PutUint64(slice, *val)
	}
	ctx.bufferLoc += 8
}

func (ctx *SerialisationContext) serialiseString(val *string) {
	if ctx.err != nil {
		return
	}

	if ctx.isReading {
		var buffer []byte
		ctx.serialiseByteSlice(&buffer)
		if buffer != nil {
			*val = string(buffer)
		}
	} else {
		buffer := []byte(*val)
		ctx.serialiseByteSlice(&buffer)
	}
}

func (ctx *SerialisationContext) serialiseStringSlice(val *[]string) {
	ctx.ensureFreeBufferSpace(2)
	if ctx.err != nil {
		return
	}

	var sliceLen uint16
	if ctx.isReading {
		ctx.serialiseUint16(&sliceLen)
		*val = make([]string, sliceLen)

	} else { // writing
		if val == nil {
			sliceLen := uint16(0)
			ctx.serialiseUint16(&sliceLen)
			return
		}

		totalLen := 2
		sliceLenInt := len(*val)
		for _, str := range *val {
			totalLen += 2 + len(str)
		}
		ctx.ensureFreeBufferSpace(totalLen)

		sliceLen = uint16(sliceLenInt)
		ctx.serialiseUint16(&sliceLen)
	}

	for i := uint16(0); i < sliceLen; i++ {
		ctx.serialiseString(&(*val)[i])
	}
}

func (ctx *SerialisationContext) serialiseByteSlice(val *[]byte) {
	ctx.ensureFreeBufferSpace(2)
	if ctx.err != nil {
		return
	}

	if ctx.isReading {
		var sliceLen uint16
		ctx.serialiseUint16(&sliceLen)
		sliceLenInt := int(sliceLen)

		ctx.ensureFreeBufferSpace(sliceLenInt)
		if ctx.err != nil {
			return
		}

		*val = make([]byte, sliceLen)
		copy(*val, ctx.buffer[ctx.bufferLoc:ctx.bufferLoc+sliceLenInt])
		ctx.bufferLoc += sliceLenInt

	} else { // writing
		sliceLenInt := 0
		if val != nil {
			sliceLenInt = len(*val)
		}
		ctx.ensureFreeBufferSpace(2 + sliceLenInt)

		var sliceLen = uint16(sliceLenInt)
		ctx.serialiseUint16(&sliceLen)
		if val != nil {
			copy(ctx.buffer[ctx.bufferLoc:], *val)
		}
		ctx.bufferLoc += sliceLenInt
	}
}

func (ctx *SerialisationContext) serialiseUint16Slice(val *[]uint16) {
	ctx.ensureFreeBufferSpace(2)
	if ctx.err != nil {
		return
	}

	if ctx.isReading {
		var sliceLen uint16
		ctx.serialiseUint16(&sliceLen)
		sliceLenInt := int(sliceLen)

		ctx.ensureFreeBufferSpace(sliceLenInt * 2)
		if ctx.err != nil {
			return
		}

		*val = make([]uint16, sliceLen)
		for i := 0; i < sliceLenInt; i++ {
			(*val)[i] = binary.LittleEndian.Uint16(ctx.buffer[ctx.bufferLoc+(2*i):])
		}
		ctx.bufferLoc += 2 * sliceLenInt

	} else { // writing
		sliceLenInt := 0
		if val != nil {
			sliceLenInt = len(*val)
		}

		ctx.ensureFreeBufferSpace(2 + 2*(sliceLenInt))
		var sliceLen = uint16(sliceLenInt)
		ctx.serialiseUint16(&sliceLen)

		if val != nil {
			for index, x := range *val {
				binary.LittleEndian.PutUint16(ctx.buffer[ctx.bufferLoc+2*index:], x)
			}
		}
		ctx.bufferLoc += 2 * sliceLenInt
	}
}

func (ctx *SerialisationContext) serialiseUint64Slice(val *[]uint64) {
	ctx.ensureFreeBufferSpace(2)
	if ctx.err != nil {
		return
	}

	if ctx.isReading {
		var sliceLen uint16
		ctx.serialiseUint16(&sliceLen)
		sliceLenInt := int(sliceLen)

		ctx.ensureFreeBufferSpace(sliceLenInt * 8)
		if ctx.err != nil {
			return
		}

		*val = make([]uint64, sliceLen)
		for i := 0; i < sliceLenInt; i++ {
			(*val)[i] = binary.LittleEndian.Uint64(ctx.buffer[ctx.bufferLoc+(8*i):])
		}
		ctx.bufferLoc += 8 * sliceLenInt

	} else { // writing
		sliceLenInt := 0
		if val != nil {
			sliceLenInt = len(*val)
		}

		ctx.ensureFreeBufferSpace(2 + 8*(sliceLenInt))
		var sliceLen = uint16(sliceLenInt)
		ctx.serialiseUint16(&sliceLen)

		if val != nil {
			for index, x := range *val {
				binary.LittleEndian.PutUint64(ctx.buffer[ctx.bufferLoc+8*index:], x)
			}
		}
		ctx.bufferLoc += 8 * sliceLenInt
	}
}

func (ctx *SerialisationContext) serialiseBool(val *bool) {
	if ctx.err != nil {
		return
	}

	if ctx.isReading {
		var temp byte
		ctx.serialiseByte(&temp)
		*val = (temp != 0)
	} else {
		var temp byte
		if *val {
			temp = 1
		} else {
			temp = 0
		}
		ctx.serialiseByte(&temp)
	}
}

func (ctx *SerialisationContext) serialiseUint16SliceSlice(val *[][]uint16) {
	ctx.ensureFreeBufferSpace(2)
	if ctx.err != nil {
		return
	}

	if ctx.isReading {
		var sliceLen uint16
		ctx.serialiseUint16(&sliceLen)
		sliceLenInt := int(sliceLen)

		*val = make([][]uint16, sliceLen)
		for i := 0; i < sliceLenInt; i++ {
			ctx.serialiseUint16Slice(&(*val)[i])
		}

	} else { // writing
		sliceLenInt := 0
		totalLen := 2
		if val != nil {
			sliceLenInt = len(*val)
			for _, slice := range *val {
				totalLen += 2 + 2*len(slice)
			}
		}
		ctx.ensureFreeBufferSpace(totalLen)

		var sliceLen = uint16(sliceLenInt)
		ctx.serialiseUint16(&sliceLen)
		if val != nil {
			for _, slice := range *val {
				ctx.serialiseUint16Slice(&slice)
			}
		}
	}
}

func (ctx *SerialisationContext) complete() error {
	if (ctx.err == nil) && (ctx.buffer != nil) && (ctx.bufferLoc != len(ctx.buffer)) {
		return ErrInvalidLength
	}
	return ctx.err
}

func (ctx *SerialisationContext) ensureBufferSize(minSize int) {
	if ctx.err != nil {
		return
	}

	if len(ctx.buffer) >= minSize {
		return
	}

	//if ctx.isReading {
	ctx.err = ErrInvalidLength
	panic("Invalid length")
	//return
	//}

	/*
		newBuffer := make([]byte, minCap)
		copy(newBuffer, ctx.buffer)
		ctx.buffer = newBuffer
	*/
}

func (ctx *SerialisationContext) ensureFreeBufferSpace(minSpace int) {
	ctx.ensureBufferSize(ctx.bufferLoc + minSpace)
}
