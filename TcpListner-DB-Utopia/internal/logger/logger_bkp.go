package logger

// import (
// 	"fmt"
// 	"io"
// 	"log"
// 	"os"
// 	"path/filepath"
// 	"runtime"
// 	"strings"
// 	"time"

// 	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
// )

// func Setup(logPath string) {

// 	err := os.MkdirAll(logPath, 0755) // Ensure the log directory exists
// 	if err != nil {
// 		log.Fatalf("Failed to create log directory: %v", err)
// 	}

// 	// Rotate logs daily, keep 7 days
// 	path := logPath + "/app.%Y-%m-%d.log"

// 	writer, err := rotatelogs.New(
// 		path,
// 		rotatelogs.WithLinkName(logPath+"/app.log"), // Always point to latest
// 		rotatelogs.WithMaxAge(7*24*time.Hour),       // Keep logs for 7 days
// 		rotatelogs.WithRotationTime(24*time.Hour),   // Rotate daily
// 	)
// 	if err != nil {
// 		log.Fatalf("Failed to initialize log file rotator: %v", err)
// 	}

// 	//  MultiWriter: log to file + terminal
// 	multiWriter := io.MultiWriter(os.Stdout, writer)

// 	log.SetOutput(multiWriter)
// 	log.SetFlags(log.Ldate | log.Ltime)
// 	log.Println("Logger initialized with daily rotation (terminal + rotating file)")
// }

// func getCaller() string {
// 	for i := 2; i < 10; i++ {
// 		_, file, line, ok := runtime.Caller(i)
// 		if !ok {
// 			break
// 		}
// 		if !strings.Contains(file, "logger") {
// 			return fmt.Sprintf("%s:%d", filepath.Base(file), line)
// 		}
// 	}
// 	return "unknown:0"
// }

// // ---------- INFO ----------
// func Info(args ...any) {
// 	log.Print(getCaller(), " [INFO] ", sprint(args...))
// }

// func Infof(format string, args ...any) {
// 	log.Printf("%s [INFO] "+format, append([]any{getCaller()}, args...)...)
// }

// // ---------- WARN ----------
// func Warn(args ...any) {
// 	log.Print(getCaller(), " [WARN] ", sprint(args...))
// }

// func Warnf(format string, args ...any) {
// 	log.Printf("%s [WARN] "+format, append([]any{getCaller()}, args...)...)
// }

// // ---------- ERROR ----------
// func Error(args ...any) {
// 	log.Print(getCaller(), " [ERROR] ", sprint(args...))
// }

// func Errorf(format string, args ...any) {
// 	log.Printf("%s [ERROR] "+format, append([]any{getCaller()}, args...)...)
// }

// // ---------- FATAL ----------
// func Fatal(args ...any) {
// 	log.Fatal(getCaller(), " [FATAL] ", sprint(args...))
// }

// func Fatalf(format string, args ...any) {
// 	log.Fatalf("%s [FATAL] "+format, append([]any{getCaller()}, args...)...)
// }

// // helper to avoid %!v noise
// func sprint(args ...any) string {
// 	if len(args) == 0 {
// 		return ""
// 	}
// 	if len(args) == 1 {
// 		return toString(args[0])
// 	}
// 	return toString(args)
// }

// func toString(v any) string {
// 	return fmt.Sprint(v)
// }

// package logger

// import (
// 	"fmt"
// 	"io"
// 	"os"
// 	"path/filepath"
// 	"runtime"
// 	"strconv"
// 	"strings"
// 	"time"

// 	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
// 	"github.com/rs/zerolog"
// )

// var log zerolog.Logger

// // Setup initializes structured logger
// func Setup(logPath string) {
// 	_ = os.MkdirAll(logPath, 0755)

// 	path := filepath.Join(logPath, "app.%Y-%m-%d.log")

// 	writer, err := rotatelogs.New(
// 		path,
// 		rotatelogs.WithLinkName(filepath.Join(logPath, "app.log")),
// 		rotatelogs.WithMaxAge(7*24*time.Hour),
// 		rotatelogs.WithRotationTime(24*time.Hour),
// 	)
// 	if err != nil {
// 		panic(err)
// 	}

// 	multi := io.MultiWriter(os.Stdout, writer)

// 	zerolog.TimeFieldFormat = time.RFC3339

// 	log = zerolog.New(multi).
// 		With().
// 		Timestamp().
// 		Logger()

// 	Info("Logger initialized (structured + rotation)")
// }

// // ---------------- CALLER ----------------
// func getCaller() string {
// 	for i := 2; i < 10; i++ {
// 		_, file, line, ok := runtime.Caller(i)
// 		if !ok {
// 			break
// 		}
// 		if !strings.Contains(file, "logger") {
// 			return filepath.Base(file) + ":" + itoa(line)
// 		}
// 	}
// 	return "unknown:0"
// }

// // fast int to string
// func itoa(i int) string {
// 	return strconv.Itoa(i)
// }

// // ---------------- INFO ----------------
// func Info(args ...any) {
// 	log.Info().
// 		Str("caller", getCaller()).
// 		Msg(sprint(args...))
// }

// func Infof(format string, args ...any) {
// 	log.Info().
// 		Str("caller", getCaller()).
// 		Msgf(format, args...)
// }

// // ---------------- WARN ----------------
// func Warn(args ...any) {
// 	log.Warn().
// 		Str("caller", getCaller()).
// 		Msg(sprint(args...))
// }

// func Warnf(format string, args ...any) {
// 	log.Warn().
// 		Str("caller", getCaller()).
// 		Msgf(format, args...)
// }

// // ---------------- ERROR ----------------
// func Error(args ...any) {
// 	log.Error().
// 		Str("caller", getCaller()).
// 		Msg(sprint(args...))
// }

// func Errorf(format string, args ...any) {
// 	log.Error().
// 		Str("caller", getCaller()).
// 		Msgf(format, args...)
// }

// // ---------------- FATAL ----------------
// func Fatal(args ...any) {
// 	log.Fatal().
// 		Str("caller", getCaller()).
// 		Msg(sprint(args...))
// }

// func Fatalf(format string, args ...any) {
// 	log.Fatal().
// 		Str("caller", getCaller()).
// 		Msgf(format, args...)
// }

// // ---------------- HELPERS ----------------
// func sprint(args ...any) string {
// 	if len(args) == 0 {
// 		return ""
// 	}
// 	if len(args) == 1 {
// 		return toString(args[0])
// 	}
// 	return toString(args)
// }

// func toString(v any) string {
// 	return fmt.Sprint(v)
// }
