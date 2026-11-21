// package logger

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
// 	"bufio"
// 	"fmt"
// 	"os"
// 	"path/filepath"
// 	"sync"
// 	"sync/atomic"
// 	"time"
// )

// const (
// 	bufferSize   = 256 * 1024 // 256KB disk buffer
// 	channelSize  = 100000     // log queue (tune based on load)
// 	flushBatch   = 1000       // batch size before flush
// 	flushTimeout = 200 * time.Millisecond
// )

// var (
// 	logDir   string
// 	logFile  *os.File
// 	writer   *bufio.Writer
// 	currDate string

// 	logChan   chan string
// 	dropCount uint64

// 	initOnce sync.Once
// )

// // -------------------- SETUP --------------------
// func Setup(path string) {
// 	initOnce.Do(func() {
// 		logDir = path
// 		_ = os.MkdirAll(logDir, 0755)

// 		logChan = make(chan string, channelSize)

// 		rotate()

// 		go writerLoop()

// 		Info("Logger initialized (EXTREME MODE)")
// 	})
// }

// // -------------------- ROTATION --------------------
// func rotate() {
// 	today := time.Now().Format("2006-01-02")

// 	if today == currDate {
// 		return
// 	}

// 	if writer != nil {
// 		writer.Flush()
// 	}
// 	if logFile != nil {
// 		logFile.Close()
// 	}

// 	fileName := filepath.Join(logDir, "app."+today+".log")

// 	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
// 	if err != nil {
// 		panic(err)
// 	}

// 	logFile = f
// 	writer = bufio.NewWriterSize(f, bufferSize)
// 	currDate = today
// }

// // -------------------- BACKGROUND WRITER --------------------
// func writerLoop() {
// 	ticker := time.NewTicker(flushTimeout)
// 	defer ticker.Stop()

// 	batch := make([]string, 0, flushBatch)

// 	for {
// 		select {
// 		case msg := <-logChan:
// 			batch = append(batch, msg)

// 			if len(batch) >= flushBatch {
// 				writeBatch(batch)
// 				batch = batch[:0]
// 			}

// 		case <-ticker.C:
// 			if len(batch) > 0 {
// 				writeBatch(batch)
// 				batch = batch[:0]
// 			}
// 		}
// 	}
// }

// // -------------------- WRITE --------------------
// func writeBatch(batch []string) {
// 	rotate()

// 	for _, line := range batch {
// 		writer.WriteString(line)
// 	}

// 	writer.Flush()

// 	// optional: console output (can disable for more speed)
// 	for _, line := range batch {
// 		fmt.Print(line)
// 	}
// }

// // -------------------- CORE LOG --------------------
// func logLine(level string, msg string) {
// 	ts := time.Now().Format("2006-01-02 15:04:05")

// 	line := ts + " [" + level + "] " + msg + "\n"

// 	select {
// 	case logChan <- line:
// 		// success
// 	default:
// 		// channel full → DROP log (critical for performance)
// 		atomic.AddUint64(&dropCount, 1)
// 	}
// }

// // -------------------- INFO --------------------
// func Info(args ...any) {
// 	logLine("INFO", sprint(args...))
// }

// func Infof(format string, args ...any) {
// 	logLine("INFO", fmt.Sprintf(format, args...))
// }

// // -------------------- WARN --------------------
// func Warn(args ...any) {
// 	logLine("WARN", sprint(args...))
// }

// func Warnf(format string, args ...any) {
// 	logLine("WARN", fmt.Sprintf(format, args...))
// }

// // -------------------- ERROR --------------------
// func Error(args ...any) {
// 	logLine("ERROR", sprint(args...))
// }

// func Errorf(format string, args ...any) {
// 	logLine("ERROR", fmt.Sprintf(format, args...))
// }

// // -------------------- FATAL --------------------
// func Fatal(args ...any) {
// 	logLine("FATAL", sprint(args...))
// 	time.Sleep(100 * time.Millisecond) // allow flush
// 	os.Exit(1)
// }

// func Fatalf(format string, args ...any) {
// 	logLine("FATAL", fmt.Sprintf(format, args...))
// 	time.Sleep(100 * time.Millisecond)
// 	os.Exit(1)
// }

// // -------------------- HELPERS --------------------
// func sprint(args ...any) string {
// 	if len(args) == 1 {
// 		return fmt.Sprint(args[0])
// 	}
// 	return fmt.Sprint(args...)
// }

package logger

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shehbaaz/db-utopia/internal/config"
)

// ===================== TUNED CONSTANTS =====================

const (
	bufferSize   = 1024 * 1024 // 1MB disk buffer
	channelSize  = 400000      // ~40MB memory (safe for your system)
	flushBatch   = 2000        // batch size
	flushTimeout = 200 * time.Millisecond
)

// ===================== GLOBALS =====================

var (
	logDir        string
	logFile       *os.File
	writer        *bufio.Writer
	currDate      string
	logChan       chan string
	dropCount     uint64
	blockOnFull   bool
	enableConsole bool

	once     sync.Once
	wg       sync.WaitGroup
	shutdown chan struct{}
)

func Setup(cfg *config.Config) {
	once.Do(func() {
		logDir = cfg.LogPath

		if strings.Contains(cfg.Env, "dev") {
			enableConsole = true
		}

		_ = os.MkdirAll(logDir, 0755)

		logChan = make(chan string, channelSize)
		shutdown = make(chan struct{})

		rotate()

		wg.Add(1)
		go writerLoop()

		Info("Logger initialized (PRODUCTION MODE)")
	})
}

func rotate() {
	today := time.Now().Format("2006-01-02")
	if today == currDate {
		return
	}

	if writer != nil {
		writer.Flush()
	}
	if logFile != nil {
		logFile.Close()
	}

	port := os.Getenv("TCP_PORT")
	if port == "" {
		port = "default"
	}

	fileName := filepath.Join(logDir, "app-"+port+"."+today+".log")

	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}

	logFile = f
	writer = bufio.NewWriterSize(f, bufferSize)
	currDate = today
}

func writerLoop() {
	defer wg.Done()

	ticker := time.NewTicker(flushTimeout)
	defer ticker.Stop()

	batch := make([]string, 0, flushBatch)

	for {
		select {
		case msg := <-logChan:
			batch = append(batch, msg)

			if len(batch) >= flushBatch {
				writeBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				writeBatch(batch)
				batch = batch[:0]
			}

		case <-shutdown:
			// flush remaining logs
			for {
				select {
				case msg := <-logChan:
					batch = append(batch, msg)
				default:
					if len(batch) > 0 {
						writeBatch(batch)
					}
					writer.Flush()
					return
				}
			}
		}
	}
}

func writeBatch(batch []string) {
	rotate()

	for _, line := range batch {
		if writer != nil {
			_, _ = writer.WriteString(line)
		}
	}

	if writer != nil {
		writer.Flush()
	}

	if enableConsole {
		for _, line := range batch {
			fmt.Print(line)
		}
	}
}

func logLine(level string, msg string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	line := ts + " [" + level + "] " + msg + "\n"

	if blockOnFull {
		logChan <- line
		return
	}

	select {
	case logChan <- line:
	default:
		atomic.AddUint64(&dropCount, 1)
	}
}

// ===================== PUBLIC =====================

func Info(args ...any)  { logLine("INFO", fmt.Sprint(args...)) }
func Warn(args ...any)  { logLine("WARN", fmt.Sprint(args...)) }
func Error(args ...any) { logLine("ERROR", fmt.Sprint(args...)) }

func Infof(format string, args ...any) {
	logLine("INFO", fmt.Sprintf(format, args...))
}

func Warnf(format string, args ...any) {
	logLine("WARN", fmt.Sprintf(format, args...))
}

func Errorf(format string, args ...any) {
	logLine("ERROR", fmt.Sprintf(format, args...))
}

func Fatal(args ...any) {
	logLine("FATAL", fmt.Sprint(args...))
	Shutdown()
	os.Exit(1)
}

func Fatalf(format string, args ...any) {
	logLine("FATAL", fmt.Sprintf(format, args...))
	Shutdown()
	os.Exit(1)
}

// ===================== SHUTDOWN =====================

var shutdownOnce sync.Once

func Shutdown() {
	shutdownOnce.Do(func() {
		close(shutdown)
		wg.Wait()
	})
}

// ===================== METRICS =====================

func DroppedLogs() uint64 {
	return atomic.LoadUint64(&dropCount)
}
