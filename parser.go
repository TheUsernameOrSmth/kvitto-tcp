package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type BongType int

const (
	BongUnknown BongType = 0
	BongKvitto  BongType = 1
	BongMessage BongType = 2
)

type BongParser struct {
	tokenizer         Tokenizer
	bongType          BongType
	kvitto            Kvitto
	message           Message
	currentCategory   string
	currentTokenIndex int
}

const (
	TF_MESSAGE                 = "10B"
	TF_HEADER_TO               = "30"
	TF_HEADER_BESTALLNING      = "20"
	TF_HEADER_KVITTONR         = "20U"
	TF_HEADER_NUMBER           = "20"
	TF_HEADER_DIVIDER          = "10"
	TF_BODY_INITIAL_BLANK_LINE = "10B"
	TF_BODY_CATEGORY           = "10CBU"
	TF_BODY_COUNT              = "20B"
	TF_BODY_PRODUCT            = "10B"
	TF_BODY_EXTRA_INFO         = "10B"
	TF_BODY_DIVIDER            = "10B"
	TF_FOOTER                  = "0B"
)

func parseMessage(message []byte) {
	currentFormat := TextFormat{
		bold:          false,
		characterSize: 0,
		underline:     UnderlineNone,
		alignment:     AlignLeft,
	}

	kvittoParser := BongParser{
		tokenizer: Tokenizer{
			tokenBlocks: make(TokenBlocks, 0),
			currentToken: TokenBlock{
				format:      currentFormat,
				text:        "",
				specialType: Text,
				rawData:     make([]uint16, 0),
			},
			reader: bytes.NewReader(message),
		},
		currentCategory:   "NOCATEGORY",
		currentTokenIndex: 0,
		kvitto: Kvitto{
			Sold: make([]SoldProduct, 0),
		},
	}
	if err := kvittoParser.parse(); err != nil {
		kvittoParser.tokenizer.print()

		log.Println("Could not create data out:", err.Error(), "raw:\n", formatHex(message))
		return
	}
	// kvittoParser.tokenizer.print()
	switch kvittoParser.bongType {
	case BongKvitto:
		kvittoParser.kvitto.publish()
	case BongMessage:
		kvittoParser.message.publish()
	default:
		log.Println("Invalid bong type:", kvittoParser.bongType)
	}
}

func (p *BongParser) parse() error {
	if err := p.tokenizer.readToEnd(); err != nil {
		return fmt.Errorf("tokenizer error %w", err)
	}

	parseErrors := make([]error, 0)

	err := p.parseMessage()
	if err == nil {
		return nil
	}
	parseErrors = append(parseErrors, err)

	err = p.parseKvitto()
	if err == nil {
		return nil
	}
	parseErrors = append(parseErrors, err)

	errorsText := "Failed to parse as known format:\n"
	for _, v := range parseErrors {
		errorsText += v.Error() + "\n"
	}
	return errors.New(errorsText)
}

func (p *BongParser) reset() {
	p.currentTokenIndex = 0
}

func (p *BongParser) readToken() *TokenBlock {
	token := p.peekToken()
	p.discardToken()
	return token
}

func (p *BongParser) readTrimmed() string {
	token := p.readToken()
	if token == nil {
		return ""
	}
	return strings.TrimSpace(token.text)
}

func (p *BongParser) peekToken() *TokenBlock {
	if len(p.tokenizer.tokenBlocks) <= p.currentTokenIndex {
		return nil
	}
	return &p.tokenizer.tokenBlocks[p.currentTokenIndex]
}
func (p *BongParser) isAtEnd() bool {
	return len(p.tokenizer.tokenBlocks) <= p.currentTokenIndex
}

func (p *BongParser) discardToken() {
	p.currentTokenIndex++
}

func (p *BongParser) expectFormat(format string) bool {
	token := p.peekToken()
	if token == nil {
		return false
	}
	if token.specialType != Text {
		return false
	}
	if token.format.shortForm() != format {
		return false
	}
	return true
}

func (p *BongParser) expectFormats(formats []string) bool {
	id := p.currentTokenIndex
	defer func() { p.currentTokenIndex = id }()
	for _, v := range formats {
		if !p.expectFormat(v) {
			return false
		}
		p.discardToken()
	}
	return true
}

func (p *BongParser) parseMessage() error {
	p.reset()
	if !p.expectFormat(TF_MESSAGE) {
		return fmt.Errorf("expected Kvitto")
	}
	message := p.readTrimmed()

	nextToken := p.peekToken()
	if nextToken == nil {
		return fmt.Errorf("early EOF")
	}
	if nextToken.specialType != FeedNLines {
		return fmt.Errorf("expected FeedNLines")
	}

	p.message.Message = message
	p.bongType = BongMessage
	return nil
}

var (
	RegexTargetPrinter = regexp.MustCompile("TILL ([a-zA-ZåäöÅÄÖ]+)")
	RegexSoldCount     = regexp.MustCompile("([0-9]+) st")
	RegexFooter        = regexp.MustCompile("Beställd av: ([a-z-A-Z]+)\nBeställd: ([0-9a-z :]+)")
)

func (p *BongParser) parseKvitto() error {
	p.reset()
	if err := p.parseHeader(); err != nil {
		return fmt.Errorf("head parse error %w", err)
	}

	if err := p.parseBody(); err != nil {
		return fmt.Errorf("body parse error %w", err)
	}

	if err := p.parseFooter(); err != nil {
		return fmt.Errorf("footer parse error %w", err)
	}

	p.bongType = BongKvitto

	return nil
}

func (p *BongParser) parseHeader() error {
	if !p.expectFormats([]string{
		TF_HEADER_TO,
		TF_HEADER_BESTALLNING,
		TF_HEADER_KVITTONR,
		TF_HEADER_NUMBER,
		TF_HEADER_DIVIDER}) {
		return errors.New("wrong format")
	}

	token := p.readToken()
	matches := RegexTargetPrinter.FindStringSubmatch(token.text)

	if len(matches) <= 1 {
		return errors.New("could not find destination")
	}
	p.kvitto.TargetPrinter = matches[1]

	p.discardToken() // "Beställning"

	kvittoName := p.readTrimmed()
	switch kvittoName {
	case "KvittoNr.":
		p.kvitto.IsBong = false
	case "Nr.":
		p.kvitto.IsBong = true
	default:
		return fmt.Errorf("invalid kvitto number prefiz: %s", kvittoName)
	}

	var err error
	p.kvitto.Number, err = strconv.Atoi(p.readTrimmed())
	if err != nil {
		return fmt.Errorf("invalid kvitto number %w", err)
	}

	p.discardToken() // "-------------"

	return nil
}

func (p *BongParser) parseBody() error {
	for {
		if p.isAtEnd() {
			return nil
		}
		if p.expectFormat(TF_FOOTER) {
			return nil // Footer;
		}
		if p.expectFormat(TF_BODY_INITIAL_BLANK_LINE) {
			p.discardToken()
		}
		if p.expectFormat(TF_BODY_CATEGORY) {
			// Category assignment
			p.currentCategory = p.readTrimmed()
		}
		if !p.expectFormats([]string{TF_BODY_COUNT, TF_BODY_PRODUCT}) {
			return errors.New("wrong format")
		}

		matches := RegexSoldCount.FindStringSubmatch(p.readTrimmed())
		if len(matches) <= 1 {
			return errors.New("cant find sold count")
		}
		var err error

		currentSold := SoldProduct{}
		currentSold.Category = p.currentCategory

		currentSold.Count, err = strconv.Atoi(matches[1])
		if err != nil {
			return err
		}
		currentSold.Name = p.readTrimmed()
		currentSold.ExtraText = ""

		if p.expectFormat(TF_BODY_EXTRA_INFO) {
			extra_text := p.readTrimmed()
			if extra_text == "-----------------------" {
				p.currentTokenIndex-- // This is a divider we have and not extra info
			} else {
				currentSold.ExtraText = extra_text
			}
		}
		if p.expectFormat(TF_BODY_DIVIDER) {
			p.discardToken() // "-----------------------"
		}
		p.kvitto.Sold = append(p.kvitto.Sold, currentSold)
	}
}

func (p *BongParser) parseFooter() error {
	if !p.expectFormat(TF_FOOTER) {
		return errors.New("wrong format")
	}

	matches := RegexFooter.FindStringSubmatch(p.readTrimmed())
	if len(matches) <= 2 {
		return errors.New("cant find seller")
	}
	loc, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		return fmt.Errorf("unable to load location: %v", err)
	}

	p.kvitto.SoldBy = matches[1]
	date := monthReplacer.Replace(matches[2])
	t, err := time.ParseInLocation("_2 Jan 15:04", date, loc)
	if err != nil {
		return fmt.Errorf("unable to parse time: %v", err)
	}
	t = t.AddDate(time.Now().Year(), 0, 0)
	p.kvitto.Time = t
	return nil
}
