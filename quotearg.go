package quotearg

import (
	"strconv"
	"unicode/utf8"
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

func QuoteargBufferRestyled(
	buffer []rune,
	arg []rune,
	style QuotingStyle,
	flags int,
	quoteTheseToo uint,
	leftQuote rune,
	rightQuote rune,
) {
	var pos int
	var elideOuterQuotes bool = (flags & QAElideOuterQuotes) != 0
	var pendingShellEscapeEnd bool
	var escaping bool
	var quoteRune rune
	var backslashEscapes bool
	var allCAndShellQuoteCompat bool = true

	store := func(c rune) {
		buffer[pos] = c
		pos++
	}

	startESC := func() {
		if elideOuterQuotes {
			goto ForceOuterQuotingStyle
		}
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
		var cAndShellQuoteCompat bool

		quoteIsNext := arg[i+1] == quoteRune

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
			break
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
			break
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

		CAndShellEscape:
			if style == ShellAlwaysQuotingStyle && elideOuterQuotes {
				goto ForceOuterQuotingStyle
			}

		CEscape:
			if backslashEscapes {
				c = esc
				goto StoreEscape
			}
			break

		case '{', '}':
			if len(arg) != 1 {
				break
			}
			fallthrough
		case '#', '~':
			if i != 0 {
				break
			}
			fallthrough
		case ' ':
			cAndShellQuoteCompat = true
			fallthrough
		case '!', '"', '$', '&', '(', ')', '*', ';', '<', '=', '>', '[':
			fallthrough
		case '^', '`', '|':
			if style == ShellAlwaysQuotingStyle && elideOuterQuotes {
				goto ForceOuterQuotingStyle
			}
			break
		case '\'':
			encounteredSingleQuote = true
			cAndShellQuoteCompat = true
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
			break
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
			cAndShellQuoteCompat = true
			break
		default:
			printable := strconv.IsPrint(c)
			cAndShellQuoteCompat = printable
			if backslashEscapes && !printable {
				startESC()
				store('x')
				switch {
				case c < ' ':
					store(byte(c) >> 4)
					store(byte(c) & 0xF)
				case c > utf8.MaxRune:
					c = 0xFFFD
					fallthrough
				case c < 0x10000:
					for s := 12; s >= 0; s -= 4 {
						store(c >> uint(s) & 0xF)
					}
				default:
					for s := 28; s >= 0; s -= 4 {
						store(c >> uint(s) & 0xF)
					}
				}
				goto StoreC
			} else if isRightQuote {
				store('\\')
				isRightQuote = false
			}
			goto StoreC
		}
	}

	if !(((backslashEscapes && style != ShellAlwaysQuotingStyle) ||
		elideOuterQuotes) && !isRightQuote) {
		goto StoreC
	}

StoreEscape:
	startESC()

StoreC:
	endESC()
	store(c)

	if !cAndShellQuoteCompat {
		allCAndShellQuoteCompat = false
	}

	if len == 0 && style == ShellAlwaysQuotingStyle && elideOuterQuotes {
		goto ForceOuterQuotingStyle
	}

ForceOuterQuotingStyle:
	if style == ShellAlwaysQuotingStyle && backslashEscapes {
		style = ShellEscapeAlwaysQuotingStyle
	}
	QuoteargBufferRestyled(
		buffer,
		arg,
		style,
		flags & ^QAElideOuterQuotes,
		0,
		leftQuote,
		rightQuote,
	)
}
