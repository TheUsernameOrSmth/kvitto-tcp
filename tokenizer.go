package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

type Alignment int

const (
	AlignLeft   Alignment = 0
	AlignCenter Alignment = 1
	AlignRight  Alignment = 2
)

type Underline int

const (
	UnderlineNone Underline = 0
	Underline1x   Underline = 1
	Underline2x   Underline = 2
)

type TextFormat struct {
	characterSize byte
	alignment     Alignment
	bold          bool
	underline     Underline
}

type SpecialBlockType int

const (
	Text       SpecialBlockType = 0
	PartialCut SpecialBlockType = 1
	FeedNLines SpecialBlockType = 2
)

func (t SpecialBlockType) String() string {
	switch t {
	case Text:
		return "Text"
	case PartialCut:
		return "PartialCut"
	case FeedNLines:
		return "FeedNLines"
	default:
		return "Unknown"
	}
}

func (t *TextFormat) shortForm() string {
	s := fmt.Sprintf("%x", t.characterSize)
	switch t.alignment {
	case AlignLeft:
	case AlignCenter:
		s += "C"
	case AlignRight:
		s += "R"
	default:
		s += "A?"
	}
	if t.bold {
		s += "B"
	}
	switch t.underline {
	case UnderlineNone:
	case Underline1x:
		s += "u"
	case Underline2x:
		s += "U"
	default:
		s += "U?"
	}
	return s
}

func (t1 *TextFormat) equals(t2 *TextFormat) bool {
	if t1.alignment != t2.alignment {
		return false
	}
	if t1.bold != t2.bold {
		return false
	}
	if t1.characterSize != t2.characterSize {
		return false
	}
	if t1.underline != t2.underline {
		return false
	}
	return true
}

type TokenBlock struct {
	format      TextFormat
	text        string
	rawData     []uint16
	specialType SpecialBlockType
}

func (t *TokenBlock) print() {
	if t.specialType == Text {
		fmt.Printf("%s: %s\n", t.format.shortForm(), strings.ReplaceAll(t.text, "\n", "\\n"))
	} else {
		fmt.Printf("[Special: %s ]\n", t.specialType)
	}
}

type TokenBlocks []TokenBlock

type Tokenizer struct {
	tokenBlocks  TokenBlocks
	currentToken TokenBlock
	reader       *bytes.Reader
}

func (t *Tokenizer) readToEnd() error {
	for {
		err := t.read()
		if err == io.EOF {
			t.pushCurrentToken()
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *Tokenizer) pushCurrentToken() {
	runes := utf16.Decode(t.currentToken.rawData)
	for _, r := range runes {
		t.currentToken.text += string(r)
	}
	if t.currentToken.text == "" && t.currentToken.specialType == Text {
		return
	}
	t.tokenBlocks = append(t.tokenBlocks, t.currentToken)
	t.currentToken.text = ""
	t.currentToken.rawData = make([]uint16, 0)
	t.currentToken.specialType = Text
}

func (t *Tokenizer) pushSpecialToken(special SpecialBlockType) {
	t.pushCurrentToken()
	t.currentToken.specialType = special
	t.pushCurrentToken()
}

func (t *Tokenizer) read() error {
	b, err := t.reader.ReadByte()
	if err != nil {
		return err
	}
	switch b {
	case 0x1b:
		fallthrough
	case 0x1c:
		fallthrough
	case 0x1d:
		t.pushCurrentToken()
		if err := t.escPosMessage(b); err != nil {
			return err
		}
	default:
		// Normal text is UTF-16 Little-endian
		secondByte, err := t.reader.ReadByte()
		if err != nil {
			return err
		}
		t.currentToken.rawData = append(t.currentToken.rawData, uint16(b)|(uint16(secondByte)<<8))
	}
	return nil
}

func (t *Tokenizer) print() {
	for _, v := range t.tokenBlocks {
		v.print()
	}
}

type EscCommand struct {
	operatorSize  int
	handleCommand func(*Tokenizer, []byte) error
}

var EscCommands = map[uint16]EscCommand{
	0x1b61: EscCommand{ // Character Size
		operatorSize: 1,
		handleCommand: func(t *Tokenizer, b []byte) error {
			switch b[0] {
			case 48:
				fallthrough
			case 0:
				t.currentToken.format.alignment = AlignLeft
				return nil
			case 49:
				fallthrough
			case 1:
				t.currentToken.format.alignment = AlignCenter
			case 50:
				fallthrough
			case 2:
				t.currentToken.format.alignment = AlignRight
			default:
				return fmt.Errorf("unknownAlign=%x", b[0])
			}
			return nil
		},
	},
	0x1b2d: EscCommand{ // Underline
		operatorSize: 1,
		handleCommand: func(t *Tokenizer, b []byte) error {
			switch b[0] {
			case 48:
				fallthrough
			case 0:
				t.currentToken.format.underline = UnderlineNone
			case 49:
				fallthrough
			case 1:
				t.currentToken.format.underline = Underline1x
			case 50:
				fallthrough
			case 2:
				t.currentToken.format.underline = Underline2x
			default:
				return fmt.Errorf("unknownUnderline=%x", b[0])
			}
			return nil
		},
	},
	0x1b45: EscCommand{ // Emphesis
		operatorSize: 1,
		handleCommand: func(t *Tokenizer, b []byte) error {
			switch b[0] {
			case 0:
				t.currentToken.format.bold = false
			case 1:
				t.currentToken.format.bold = true
			default:
				return fmt.Errorf("unknownEmphesis=%x", b[0])
			}
			return nil
		},
	},
	0x1b64: EscCommand{ // Feed n lines
		operatorSize: 1,
		handleCommand: func(t *Tokenizer, b []byte) error {
			t.pushSpecialToken(FeedNLines)
			return nil
		},
	},
	0x1b69: EscCommand{ // Partial cut
		operatorSize: 0,
		handleCommand: func(t *Tokenizer, b []byte) error {
			t.pushSpecialToken(PartialCut)
			return nil
		},
	},
	0x1d21: EscCommand{
		operatorSize: 1,
		handleCommand: func(t *Tokenizer, b []byte) error {

			t.currentToken.format.characterSize = b[0]
			return nil
		},
	},
}

func (t *Tokenizer) escPosMessage(firstByte byte) error {
	secondByte, err := t.reader.ReadByte()
	if err != nil {
		return err
	}

	commandId := binary.BigEndian.Uint16([]byte{firstByte, secondByte})
	commandOperator, ok := EscCommands[commandId]
	if !ok {
		// Note: Most commands are 3 bytes,
		// thus if it is desired to not give up
		//
		//_, _ = t.reader.ReadByte()
		return fmt.Errorf("unknown_%x", commandId)
	} else {
		operands := []byte{}
		for range commandOperator.operatorSize {
			operand, err := t.reader.ReadByte()
			if err != nil {
				return fmt.Errorf("command operand reading error: %w", err)
			}
			operands = append(operands, operand)
		}
		if err := commandOperator.handleCommand(t, operands); err != nil {
			return err
		}
	}

	return nil
}
