package main

import (
	"fmt"

	"github.com/go-hep/fwk"
)

type task2 struct {
	fwk.TaskBase

	fct func(f float64) float64
}

func (tsk *task2) Configure(ctx fwk.Context) fwk.Error {
	var err fwk.Error
	fmt.Printf(">>> configure [%v]...\n", tsk.CompName())

	tsk.fct = func(f float64) float64 {
		return f * f
	}

	err = tsk.DeclProp("Fct", &tsk.fct)
	if err != nil {
		return err
	}

	err = tsk.DeclInPort("floats1")
	if err != nil {
		return err
	}

	err = tsk.DeclOutPort("massaged_floats1")
	if err != nil {
		return err
	}

	fmt.Printf(">>> configure [%v]... [done]\n", tsk.CompName())
	return err
}

func (tsk *task2) StartTask(ctx fwk.Context) fwk.Error {
	fmt.Printf(">>> start [%v]...\n", tsk.CompName())
	return nil
}

func (tsk *task2) StopTask(ctx fwk.Context) fwk.Error {
	fmt.Printf(">>> stop [%v]...\n", tsk.CompName())
	return nil
}

func (tsk *task2) Process(ctx fwk.Context) fwk.Error {
	fmt.Printf(">>> proc [%v]...\n", tsk.CompName())
	store := ctx.Store()
	v, err := store.Get("floats1")
	if err != nil {
		return err
	}
	v = tsk.fct(v.(float64))
	err = store.Put("massaged_floats1", v)
	if err != nil {
		return err
	}
	return nil
}
