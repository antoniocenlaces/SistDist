package logger

import (
	"fmt"
	"os"
	"time"
)

type Logger struct {
	pid int
}

// Variable global exportada
var Log *Logger

// Inicializa el logger
func init() {
	Log = &Logger{
		pid: os.Getpid(),
	}
}

// método interno que acepta múltiples valores y los imprime en consola
func (l *Logger) log(level string, v ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	prefix := fmt.Sprintf("[%s] [PID:%d] [%s]", timestamp, l.pid, level)
	msg := fmt.Sprintln(v...)
	finalMsg := fmt.Sprintf("%s %s", prefix, msg)

	fmt.Print(finalMsg)
}

// Métodos públicos
func (l *Logger) Info(v ...interface{}) {
	l.log("INFO", v...)
}

func (l *Logger) Warning(v ...interface{}) {
	l.log("WARNING", v...)
}

func (l *Logger) Error(v ...interface{}) {
	l.log("ERROR", v...)
}
