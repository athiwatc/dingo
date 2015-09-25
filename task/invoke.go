package task

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/satori/go.uuid"
)

//
// errors
//

type Invoker interface {
	Invoke(f interface{}, param []interface{}) ([]interface{}, error)
	ComposeTask(name string, args ...interface{}) (Task, error)
}

type _invoker struct {
}

//
// factory function
//
func NewDefaultInvoker() Invoker {
	return &_invoker{}
}

//
// private function
//

func (vk *_invoker) convert2slice(v, r reflect.Value, rt reflect.Type) (err error) {
	if v.Kind() != reflect.Slice {
		err = errors.New(fmt.Sprintf("Only Slice not %v convertible to slice", v.Kind().String()))
		return
	}

	r.Set(reflect.MakeSlice(rt, 0, v.Len()))
	for i := 0; i < v.Len(); i++ {
		converted, err_ := vk.convert(v.Index(i), rt.Elem())
		if err_ != nil {
			err = err_
			break
		} else {
			r.Set(reflect.Append(r, converted))
		}
	}

	return
}

func (vk *_invoker) convert2map(v, r reflect.Value, rt reflect.Type) (err error) {
	if v.Kind() != reflect.Map {
		err = errors.New(fmt.Sprintf("Only Map not %v convertible to map", v.Kind().String()))
		return
	}

	// init map
	r.Set(reflect.MakeMap(rt))

	keys := v.MapKeys()
	for _, k := range keys {
		converted, err_ := vk.convert(v.MapIndex(k), rt.Elem())
		if err_ != nil {
			// propagate error
			err = err_
			break
		} else {
			r.SetMapIndex(k, converted)
		}
	}

	return
}

func (vk *_invoker) convert2struct(v, r reflect.Value, rt reflect.Type) (err error) {
	if v.Kind() != reflect.Map {
		err = errors.New(fmt.Sprintf("Only Map not %v convertible to struct", v.Kind().String()))
		return
	}
	for i := 0; i < r.NumField(); i++ {
		fv := r.Field(i)
		if !fv.CanSet() {
			continue
		}

		var converted reflect.Value

		ft := rt.Field(i)
		if ft.Anonymous {
			converted, err = vk.convert(v, ft.Type)
		} else {
			// json tags
			// TODO: move this to a private function
			key := ft.Tag.Get("json")
			if key != "" {
				key = strings.Trim(key, "\"")
				keys := strings.Split(key, ",")
				key = keys[0]
				if key == "-" {
					key = ""
				}
			}
			if key == "" {
				key = ft.Name
			}
			mv := v.MapIndex(reflect.ValueOf(key))
			if !mv.IsValid() {
				err = errors.New(fmt.Sprintf("Invalid value returned from map by key: %v", ft))
				break
			}

			converted, err = vk.convert(mv, fv.Type())
		}

		if err != nil {
			break
		}

		fv.Set(converted)
	}

	return err
}

func (vk *_invoker) convert(v reflect.Value, t reflect.Type) (reflect.Value, error) {
	var err error

	if v.IsValid() {
		if v.Type().Kind() == reflect.Interface {
			// type assertion
			// by convert to interface{} and reflect it
			val := v.Interface()
			if val == nil {
				if t.Kind() != reflect.Ptr {
					err = errors.New("Can't pass nil for non-ptr parameter")
				}
				// for pointer type, reflect.Zero create a nil pointer
				return reflect.Zero(t), err
			}

			v = reflect.ValueOf(val)
		}
		if v.Type().ConvertibleTo(t) {
			return v.Convert(t), nil
		}
	}

	deref := 0
	// only reflect.Value from reflect.New is settable
	// reflect.Zero is not.
	ret := reflect.New(t)
	for ; t.Kind() == reflect.Ptr; deref++ {
		t = t.Elem()
		ret.Elem().Set(reflect.New(t))
		ret = ret.Elem()
	}
	elm := ret.Elem()

	switch elm.Kind() {
	case reflect.Struct:
		err = vk.convert2struct(v, elm, t)
	case reflect.Map:
		err = vk.convert2map(v, elm, t)
	case reflect.Slice:
		err = vk.convert2slice(v, elm, t)
	default:
		err = errors.New(fmt.Sprintf("Unsupported Element Type: %v", elm.Kind().String()))
	}

	if deref == 0 {
		ret = ret.Elem()
	} else {
		// derefencing to correct type
		for deref--; deref > 0; deref-- {
			ret = ret.Addr()
		}
	}

	return ret, err
}

//
// helper function for converting a value based on a type
//
func (vk *_invoker) from(val interface{}, t reflect.Type) (reflect.Value, error) {
	if val == nil {
		var err error

		if t.Kind() != reflect.Ptr {
			err = errors.New("Can't pass nil for non-ptr parameter")
		}
		// for pointer type, reflect.Zero create a nil pointer
		return reflect.Zero(t), err
	}

	return vk.convert(reflect.ValueOf(val), t)
}

//
// Invoker interface
//

//
// reference implementation
//	  https://github.com/codegangsta/inject/blob/master/inject.go
//
func (vk *_invoker) Invoke(f interface{}, param []interface{}) ([]interface{}, error) {
	var err error

	funcT := reflect.TypeOf(f)

	// make sure parameter matched
	if len(param) != funcT.NumIn() {
		return nil, errors.New(fmt.Sprintf("Parameter Count mismatch: %v %v", len(param), funcT.NumIn()))
	}

	// convert param into []reflect.Value
	var in = make([]reflect.Value, funcT.NumIn())
	for i := 0; i < funcT.NumIn(); i++ {
		in[i], err = vk.from(param[i], funcT.In(i))
		if err != nil {
			return nil, err
		}
	}

	// invoke the function
	ret := reflect.ValueOf(f).Call(in)
	out := make([]interface{}, funcT.NumOut())

	// we don't have to check the count of return values and out,
	// if not match, it won't build

	for i := 0; i < funcT.NumOut(); i++ {
		if ret[i].CanInterface() {
			out[i] = ret[i].Interface()
		}
	}

	return out, nil
}

func (vk *_invoker) ComposeTask(name string, args ...interface{}) (Task, error) {
	return &_task{
		Id:   uuid.NewV4().String(),
		Name: name,
		Args: args,
	}, nil
}