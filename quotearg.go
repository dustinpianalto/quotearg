package quotearg

import (
	"strconv"
	"unicode/utf8"
)

var (
	lowerHex = []rune{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f'}
	upperHex = []rune{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'A', 'B', 'C', 'D', 'E', 'F'}
)

const (
	QAElideNullBytes   = 0x01
	QAElideOuterQuotes = 0x02
	QASplitTrigraphs   = 0x04
)

const SizeMax = ^uint(0)

type QuotingOptions struct {
	Flags         int
	QuoteTheseToo uint
	LeftQuote     rune
	RightQuote    rune
	Style         QuotingStyle
}

type QuotingStyle int

const (
	LiteralQuotingStyle QuotingStyle = iota
	ShellQuotingStyle
	ShellAlwaysQuotingStyle
	ShellEscapeQuotingStyle
	ShellEscapeAlwaysQuotingStyle
	CQuotingStyle
	CMaybeQuotingStyle
	EscapeQuotingStyle
	LocaleQuotingStyle
	CLocaleQuotingStyle
	CustomQuotingStyle
)

var QuotingStyleArgs = []string{
	"literal",
	"shell",
	"shell-always",
	"shell-escape",
	"shell-escape-always",
	"c",
	"c-maybe",
	"escape",
	"locale",
	"clocale",
	"",
}

func GetTextQuote(s QuotingStyle) rune {
	if s == CLocaleQuotingStyle {
		return '\''
	} else {
		return '"'
	}
}

func Quote(
	buffer []rune,
	arg []rune,
	style QuotingStyle,
	flags int,
	quoteTheseToo uint,
	leftQuote rune,
	rightQuote rune,
) []rune {
	var elideOuterQuotes bool = (flags & QAElideOuterQuotes) != 0
	var pendingShellEscapeEnd bool
	var escaping bool
	var quoteRune rune
	var backslashEscapes bool

	store := func(c rune) {
		buffer = append(buffer, c)
	}

	startESC := func() {
		escaping = true
		if style == ShellAlwaysQuotingStyle && !pendingShellEscapeEnd {
			store('\'')
			store('$')
			store('\'')
			pendingShellEscapeEnd = true
		}
		store('\\')
	}

	endESC := func() {
		if pendingShellEscapeEnd && !escaping {
			store('\'')
			store('\'')
			pendingShellEscapeEnd = false
		}
	}

	switch style {
	case CMaybeQuotingStyle:
		style = CQuotingStyle
		elideOuterQuotes = true
		fallthrough
	case CQuotingStyle:
		if !elideOuterQuotes {
			store('"')
		}
		backslashEscapes = true
		quoteRune = '"'
		break
	case EscapeQuotingStyle:
		backslashEscapes = true
		elideOuterQuotes = false
		break
	case LocaleQuotingStyle:
		fallthrough
	case CLocaleQuotingStyle:
		fallthrough
	case CustomQuotingStyle:
		if style != CustomQuotingStyle {
			leftQuote = GetTextQuote(style)
			rightQuote = GetTextQuote(style)
		}
		if !elideOuterQuotes {
			// todo figure out the voodoo
		}
		backslashEscapes = true
		quoteRune = rightQuote
		break
	case ShellEscapeQuotingStyle:
		backslashEscapes = true
		fallthrough
	case ShellQuotingStyle:
		elideOuterQuotes = true
		fallthrough
	case ShellEscapeAlwaysQuotingStyle:
		if !elideOuterQuotes {
			backslashEscapes = true
		}
		fallthrough
	case ShellAlwaysQuotingStyle:
		style = ShellAlwaysQuotingStyle
		if !elideOuterQuotes {
			store('\'')
		}
		quoteRune = '\''
		break
	default:
		panic("invalid style")
	}

	for i := 0; i < len(arg); i++ {
		var esc rune
		var isRightQuote bool
		escaping = false

		quoteIsNext := len(arg) > i+1 && arg[i+1] == quoteRune

		if backslashEscapes &&
			style != ShellAlwaysQuotingStyle &&
			quoteIsNext {
			if elideOuterQuotes {
				goto ForceOuterQuotingStyle
			}
			isRightQuote = true
		}

		c := arg[i]

		switch c {
		case 0x00:
			if backslashEscapes {
				if elideOuterQuotes {
					goto ForceOuterQuotingStyle
				}
				startESC()
				if style != ShellQuotingStyle &&
					i+1 < len(arg) &&
					'0' <= arg[i+1] &&
					arg[i+1] <= '9' {
					store('0')
					store('0')
				}
				c = '0'
			} else if flags&QAElideNullBytes != 0 {
				continue
			}
			goto StoreEscape
		case '?':
			switch style {
			case ShellAlwaysQuotingStyle:
				if elideOuterQuotes {
					goto ForceOuterQuotingStyle
				}
				break
			case CQuotingStyle:
				if flags&QASplitTrigraphs != 0 &&
					i+2 < len(arg) &&
					arg[i+1] == '?' {
					switch arg[i+2] {
					case '!', '\'', '(', ')', '-', '/', '<', '=', '>':
						if elideOuterQuotes {
							goto ForceOuterQuotingStyle
						}

						c = arg[i+2]
						i += 2
						store('?')
						store('"')
						store('"')
						store('?')
						break
					default:
						break
					}
				}
				break
			default:
				break
			}
			goto StoreEscape
		case '\a':
			esc = 'a'
			goto CEscape
		case '\b':
			esc = 'b'
			goto CEscape
		case '\f':
			esc = 'f'
			goto CEscape
		case '\n':
			esc = 'n'
			goto CAndShellEscape
		case '\r':
			esc = 'r'
			goto CAndShellEscape
		case '\t':
			esc = 't'
			goto CAndShellEscape
		case '\v':
			esc = 'v'
			goto CEscape
		case '\\':
			esc = c
			if style == ShellAlwaysQuotingStyle {
				if elideOuterQuotes {
					goto ForceOuterQuotingStyle
				}
				goto StoreC
			}

			if backslashEscapes && elideOuterQuotes && quoteRune > 0 {
				goto StoreC
			}

			goto CAndShellEscape

		case '{', '}':
			if len(arg) != 1 {
				goto StoreEscape
			}
			fallthrough
		case '#', '~':
			if i != 0 {
				goto StoreEscape
			}
			fallthrough
		case ' ':
			fallthrough
		case '!', '"', '$', '&', '(', ')', '*', ';', '<', '=', '>', '[':
			fallthrough
		case '^', '`', '|':
			if style == ShellAlwaysQuotingStyle && elideOuterQuotes {
				goto ForceOuterQuotingStyle
			}
			goto StoreEscape
		case '\'':
			if style == ShellAlwaysQuotingStyle {
				if elideOuterQuotes {
					goto ForceOuterQuotingStyle
				}
				// buffersize voodoo

				store('\'')
				store('\\')
				store('\'')
				pendingShellEscapeEnd = false
			}
			goto StoreC
		case '%', '+', ',', '-', '.', '/', '0', '1', '2', '3', '4', '5':
			fallthrough
		case '6', '7', '8', '9', ':', 'A', 'B', 'C', 'D', 'E', 'F':
			fallthrough
		case 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R':
			fallthrough
		case 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z', ']', '_', 'a', 'b':
			fallthrough
		case 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n':
			fallthrough
		case 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z':
			goto StoreC
		default:
			if backslashEscapes && !strconv.IsPrint(c) {
				if elideOuterQuotes {
					goto ForceOuterQuotingStyle
				}
				switch {
				case c < ' ':
					startESC()
					store('x')
					store(upperHex[byte(c)>>4])
					store(upperHex[byte(c)&0xF])
				case c > utf8.MaxRune:
					c = 0xFFFD
					fallthrough
				case c < 0x10000:
					startESC()
					store('u')
					for s := 12; s >= 0; s -= 4 {
						store(upperHex[c>>uint(s)&0xF])
					}
				default:
					startESC()
					store('U')
					for s := 28; s >= 0; s -= 4 {
						store(upperHex[c>>uint(s)&0xF])
					}
				}
			} else if isRightQuote {
				store('\\')
				isRightQuote = false
			}
		}
		continue
	CAndShellEscape:
		if style == ShellAlwaysQuotingStyle && elideOuterQuotes {
			goto ForceOuterQuotingStyle
		}

	CEscape:
		if backslashEscapes {
			c = esc
			goto StoreEscape
		}

		if !(((backslashEscapes && style != ShellAlwaysQuotingStyle) ||
			elideOuterQuotes) && !isRightQuote) {
			goto StoreC
		}

	StoreEscape:
		if elideOuterQuotes {
			goto ForceOuterQuotingStyle
		}
		startESC()

	StoreC:
		endESC()
		store(c)

	}

	if quoteRune != 0 && !elideOuterQuotes {
		store(quoteRune)
	}

	if style == ShellAlwaysQuotingStyle && elideOuterQuotes {
		goto ForceOuterQuotingStyle
	}
	return buffer

ForceOuterQuotingStyle:
	if style == ShellAlwaysQuotingStyle && backslashEscapes {
		style = ShellEscapeAlwaysQuotingStyle
	}
	return Quote(
		buffer,
		arg,
		style,
		flags & ^QAElideOuterQuotes,
		0,
		leftQuote,
		rightQuote,
	)
}
