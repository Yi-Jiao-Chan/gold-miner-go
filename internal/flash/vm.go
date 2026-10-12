package flash

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

func number(v any) float64 {
	switch x := v.(type) {
	case nil:
		return 0
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case uint32:
		return float64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		if strings.TrimSpace(x) == "" {
			return 0
		}
		n, e := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if e == nil {
			return n
		}
		return math.NaN()
	}
	return math.NaN()
}
func stringValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "undefined"
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case *Node:
		if x == nil {
			return "undefined"
		}
		return x.Name
	default:
		return fmt.Sprint(v)
	}
}
func truth(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case *Node:
		return x != nil
	default:
		n := number(v)
		return n != 0 && !math.IsNaN(n)
	}
}
func equal(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if x, ok := a.(*Node); ok {
		y, _ := b.(*Node)
		return x == y
	}
	if x, ok := a.(string); ok {
		if y, ok := b.(string); ok {
			return x == y
		}
	}
	return number(a) == number(b)
}
func less(a, b any) bool {
	x, xok := a.(string)
	y, yok := b.(string)
	if xok && yok {
		return x < y
	}
	return number(a) < number(b)
}

type machine struct {
	stack []any
	pool  []string
}

func (m *machine) push(v any) { m.stack = append(m.stack, v) }
func (m *machine) pop() (any, error) {
	if len(m.stack) == 0 {
		return nil, fmt.Errorf("action stack underflow")
	}
	v := m.stack[len(m.stack)-1]
	m.stack = m.stack[:len(m.stack)-1]
	return v, nil
}
func cstring(data []byte, at *int) (string, error) {
	start := *at
	for *at < len(data) && data[*at] != 0 {
		*at++
	}
	if *at >= len(data) {
		return "", fmt.Errorf("unterminated action string")
	}
	s := string(data[start:*at])
	*at++
	return s, nil
}
func (m *machine) pushData(data []byte) error {
	for i := 0; i < len(data); {
		kind := data[i]
		i++
		need := func(n int) error {
			if i+n > len(data) {
				return fmt.Errorf("truncated push value %d", kind)
			}
			return nil
		}
		switch kind {
		case 0:
			s, err := cstring(data, &i)
			if err != nil {
				return err
			}
			m.push(s)
		case 1:
			if e := need(4); e != nil {
				return e
			}
			m.push(float64(math.Float32frombits(binary.LittleEndian.Uint32(data[i:]))))
			i += 4
		case 2, 3:
			m.push(nil)
		case 5:
			if e := need(1); e != nil {
				return e
			}
			m.push(data[i] != 0)
			i++
		case 6:
			if e := need(8); e != nil {
				return e
			}
			bits := uint64(binary.LittleEndian.Uint32(data[i:]))<<32 | uint64(binary.LittleEndian.Uint32(data[i+4:]))
			m.push(math.Float64frombits(bits))
			i += 8
		case 7:
			if e := need(4); e != nil {
				return e
			}
			m.push(float64(int32(binary.LittleEndian.Uint32(data[i:]))))
			i += 4
		case 8, 9:
			width := 1
			if kind == 9 {
				width = 2
			}
			if e := need(width); e != nil {
				return e
			}
			index := int(data[i])
			if width == 2 {
				index = int(binary.LittleEndian.Uint16(data[i:]))
			}
			i += width
			if index >= len(m.pool) {
				return fmt.Errorf("constant pool index %d", index)
			}
			m.push(m.pool[index])
		default:
			return fmt.Errorf("unsupported push type %d", kind)
		}
	}
	return nil
}
func (r *Runtime) run(n *Node, code []byte) error {
	previous := r.playerContext
	r.playerContext = n.player
	defer func() { r.playerContext = previous }()
	m := &machine{}
	for pc, steps := 0, 0; pc < len(code); steps++ {
		if steps >= 100000 {
			return fmt.Errorf("action instruction limit")
		}
		at := pc
		op := code[pc]
		pc++
		var data []byte
		if op >= 0x80 {
			if pc+2 > len(code) {
				return fmt.Errorf("truncated action %02x", op)
			}
			length := int(binary.LittleEndian.Uint16(code[pc:]))
			pc += 2
			if pc+length > len(code) {
				return fmt.Errorf("truncated action body %02x", op)
			}
			data = code[pc : pc+length]
			pc += length
		}
		pop := func() any {
			v, e := m.pop()
			if e != nil {
				panic(e)
			}
			return v
		}
		// Recover malformed stack data as a VM error, without hiding runtime bugs.
		err := func() (err error) {
			defer func() {
				if v := recover(); v != nil {
					if e, ok := v.(error); ok {
						err = e
					} else {
						panic(v)
					}
				}
			}()
			switch op {
			case 0x00:
				return nil
			case 0x04:
				return r.seek(n, n.Frame+1, false)
			case 0x06:
				n.Playing = true
			case 0x07:
				n.Playing = false
			case 0x0b:
				b, a := number(pop()), number(pop())
				m.push(a - b)
			case 0x0c:
				b, a := number(pop()), number(pop())
				m.push(a * b)
			case 0x0d:
				b, a := number(pop()), number(pop())
				m.push(a / b)
			case 0x10:
				b, a := truth(pop()), truth(pop())
				m.push(a && b)
			case 0x11:
				b, a := truth(pop()), truth(pop())
				m.push(a || b)
			case 0x12:
				m.push(!truth(pop()))
			case 0x13:
				b, a := stringValue(pop()), stringValue(pop())
				m.push(a == b)
			case 0x17:
				pop()
			case 0x18:
				v := number(pop())
				if math.IsNaN(v) || math.IsInf(v, 0) {
					v = 0
				}
				m.push(math.Trunc(v))
			case 0x1c:
				m.push(r.Get(n, stringValue(pop())))
			case 0x1d:
				value, path := pop(), stringValue(pop())
				r.Set(n, path, value)
			case 0x22:
				index, target := int(number(pop())), r.resolve(n, pop())
				if index < 0 || index >= len(propertyNames) {
					return fmt.Errorf("unknown property %d", index)
				}
				m.push(r.member(target, propertyNames[index]))
			case 0x23:
				value, index, target := pop(), int(number(pop())), r.resolve(n, pop())
				if index < 0 || index >= len(propertyNames) {
					return fmt.Errorf("unknown property %d", index)
				}
				r.setMember(target, propertyNames[index], value)
			case 0x25:
				target := r.resolve(n, pop())
				if target != nil && target.Parent != nil {
					for d, c := range target.Parent.Children {
						if c == target {
							delete(target.Parent.Children, d)
						}
					}
				}
			case 0x30:
				max := int(number(pop()))
				if max <= 0 {
					m.push(float64(0))
				} else {
					m.push(float64(r.rng.IntN(max)))
				}
			case 0x47:
				b, a := pop(), pop()
				_, as := a.(string)
				_, bs := b.(string)
				if as || bs {
					m.push(stringValue(a) + stringValue(b))
				} else {
					m.push(number(a) + number(b))
				}
			case 0x48:
				b, a := pop(), pop()
				m.push(less(a, b))
			case 0x49:
				b, a := pop(), pop()
				m.push(equal(a, b))
			case 0x4a:
				m.push(number(pop()))
			case 0x4e:
				key, obj := stringValue(pop()), pop()
				target, _ := obj.(*Node)
				m.push(r.member(target, key))
			case 0x4f:
				value, key, obj := pop(), stringValue(pop()), pop()
				target, _ := obj.(*Node)
				r.setMember(target, key, value)
			case 0x50:
				m.push(number(pop()) + 1)
			case 0x52:
				name, obj, count := stringValue(pop()), pop(), int(number(pop()))
				if count < 0 || count > len(m.stack) {
					return fmt.Errorf("invalid method argument count %d", count)
				}
				args := make([]any, count)
				for i := range args {
					args[i] = pop()
				}
				result, e := r.call(n, obj, name, args)
				if e != nil {
					return e
				}
				m.push(result)
			case 0x81:
				if len(data) != 2 {
					return fmt.Errorf("invalid GotoFrame")
				}
				// GotoFrame stops; gotoAndPlay appends a separate ActionPlay.
				return r.seek(n, int(binary.LittleEndian.Uint16(data))+1, false)
			case 0x83:
				i := 0
				url, e := cstring(data, &i)
				if e != nil {
					return e
				}
				_, e = cstring(data, &i)
				if e != nil {
					return e
				}
				r.BlockedURLs = append(r.BlockedURLs, url)
			case 0x88:
				if len(data) < 2 {
					return fmt.Errorf("invalid ConstantPool")
				}
				count := int(binary.LittleEndian.Uint16(data))
				i := 2
				m.pool = nil
				for j := 0; j < count; j++ {
					s, e := cstring(data, &i)
					if e != nil {
						return e
					}
					m.pool = append(m.pool, s)
				}
			case 0x96:
				return m.pushData(data)
			case 0x99, 0x9d:
				if len(data) != 2 {
					return fmt.Errorf("invalid branch")
				}
				branch := true
				if op == 0x9d {
					branch = truth(pop())
				}
				if branch {
					pc += int(int16(binary.LittleEndian.Uint16(data)))
					if pc < 0 || pc > len(code) {
						return fmt.Errorf("branch outside action block")
					}
				}
			case 0x9a:
				pop()
				url := stringValue(pop())
				r.BlockedURLs = append(r.BlockedURLs, url)
			case 0x9f:
				if len(data) < 1 {
					return fmt.Errorf("invalid GotoFrame2")
				}
				value := pop()
				frame := r.frame(n, value)
				if data[0]&2 != 0 {
					if len(data) != 3 {
						return fmt.Errorf("invalid scene bias")
					}
					frame += int(binary.LittleEndian.Uint16(data[1:]))
				}
				return r.seek(n, frame, data[0]&1 != 0)
			default:
				return fmt.Errorf("unsupported AVM1 opcode 0x%02x at %d", op, at)
			}
			return nil
		}()
		if err != nil {
			return err
		}
		if op == 0 {
			return nil
		}
	}
	return nil
}
func (r *Runtime) call(context *Node, obj any, name string, args []any) (any, error) {
	n := r.resolve(context, obj)
	if n == nil {
		return nil, nil
	}
	arg := func(i int) any {
		if i >= len(args) {
			return nil
		}
		return args[i]
	}
	switch strings.ToLower(name) {
	case "gotoandplay":
		return nil, r.seek(n, r.frame(n, arg(0)), true)
	case "gotoandstop":
		return nil, r.seek(n, r.frame(n, arg(0)), false)
	case "nextframe":
		return nil, r.seek(n, n.Frame+1, false)
	case "prevframe":
		return nil, r.seek(n, n.Frame-1, false)
	case "play":
		n.Playing = true
	case "stop":
		n.Playing = false
	case "getbytesloaded", "getbytestotal":
		return float64(298548), nil
	case "hittest":
		return intersects(r.Bounds(n), r.Bounds(r.resolve(context, arg(0)))), nil
	default:
		return nil, fmt.Errorf("unsupported movie clip method %q", name)
	}
	return nil, nil
}

// String formats an AVM1 value for dynamic text.
func String(v any) string { return stringValue(v) }
