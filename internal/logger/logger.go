package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

type Logger struct {
	infoLogger  *log.Logger
	errorLogger *log.Logger
	debugLogger *log.Logger
	logDir      string
	infoFile    *os.File
	errorFile   *os.File
	debugFile   *os.File
}

func NewLogger(logDir string) (*Logger, error) {
	// 创建日志目录
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %v", err)
	}

	logger := &Logger{
		logDir: logDir,
	}

	// 初始化日志文件
	if err := logger.initLogFiles(); err != nil {
		return nil, err
	}

	// 启动日志清理协程
	go logger.startLogCleanup()

	return logger, nil
}

func (l *Logger) initLogFiles() error {
	now := time.Now()
	dateStr := now.Format("2006-01-02")

	// 信息日志文件
	infoPath := filepath.Join(l.logDir, fmt.Sprintf("info_%s.log", dateStr))
	infoFile, err := os.OpenFile(infoPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("创建信息日志文件失败: %v", err)
	}
	l.infoFile = infoFile

	// 错误日志文件
	errorPath := filepath.Join(l.logDir, fmt.Sprintf("error_%s.log", dateStr))
	errorFile, err := os.OpenFile(errorPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("创建错误日志文件失败: %v", err)
	}
	l.errorFile = errorFile

	// 调试日志文件
	debugPath := filepath.Join(l.logDir, fmt.Sprintf("debug_%s.log", dateStr))
	debugFile, err := os.OpenFile(debugPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("创建调试日志文件失败: %v", err)
	}
	l.debugFile = debugFile

	// 创建多输出writer（同时输出到文件和控制台）
	infoWriter := io.MultiWriter(os.Stdout, l.infoFile)
	errorWriter := io.MultiWriter(os.Stderr, l.errorFile)
	debugWriter := io.MultiWriter(os.Stdout, l.debugFile)

	// 初始化logger
	l.infoLogger = log.New(infoWriter, "[INFO] ", log.LstdFlags|log.Lshortfile)
	l.errorLogger = log.New(errorWriter, "[ERROR] ", log.LstdFlags|log.Lshortfile)
	l.debugLogger = log.New(debugWriter, "[DEBUG] ", log.LstdFlags|log.Lshortfile)

	return nil
}

func (l *Logger) Info(format string, v ...interface{}) {
	l.infoLogger.Printf(format, v...)
}

func (l *Logger) Error(format string, v ...interface{}) {
	l.errorLogger.Printf(format, v...)
}

func (l *Logger) Debug(format string, v ...interface{}) {
	l.debugLogger.Printf(format, v...)
}

func (l *Logger) InfoWithContext(context string, format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	l.infoLogger.Printf("[%s] %s", context, message)
}

func (l *Logger) ErrorWithContext(context string, format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	l.errorLogger.Printf("[%s] %s", context, message)
}

func (l *Logger) DebugWithContext(context string, format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	l.debugLogger.Printf("[%s] %s", context, message)
}

// startLogCleanup 启动日志清理协程，每48小时删除旧日志
func (l *Logger) startLogCleanup() {
	ticker := time.NewTicker(48 * time.Hour)
	defer ticker.Stop()

	// 立即执行一次清理
	l.cleanupOldLogs()

	for range ticker.C {
		l.cleanupOldLogs()
	}
}

// cleanupOldLogs 删除48小时前的日志文件
func (l *Logger) cleanupOldLogs() {
	cutoffTime := time.Now().Add(-48 * time.Hour)
	
	files, err := filepath.Glob(filepath.Join(l.logDir, "*.log"))
	if err != nil {
		l.Error("扫描日志文件失败: %v", err)
		return
	}

	deletedCount := 0
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoffTime) {
			if err := os.Remove(file); err != nil {
				l.Error("删除旧日志文件失败 %s: %v", file, err)
			} else {
				deletedCount++
				l.Info("已删除旧日志文件: %s", filepath.Base(file))
			}
		}
	}

	if deletedCount > 0 {
		l.Info("日志清理完成，删除了 %d 个旧日志文件", deletedCount)
	}
}

// Close 关闭日志文件
func (l *Logger) Close() error {
	var errors []error
	
	if l.infoFile != nil {
		if err := l.infoFile.Close(); err != nil {
			errors = append(errors, err)
		}
	}
	
	if l.errorFile != nil {
		if err := l.errorFile.Close(); err != nil {
			errors = append(errors, err)
		}
	}
	
	if l.debugFile != nil {
		if err := l.debugFile.Close(); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("关闭日志文件时发生错误: %v", errors)
	}
	
	return nil
}

// LogGameAction 记录游戏相关操作
func (l *Logger) LogGameAction(userID int64, action string, details string) {
	l.InfoWithContext("GAME", "用户 %d 执行操作: %s - %s", userID, action, details)
}

// LogDatabaseAction 记录数据库操作
func (l *Logger) LogDatabaseAction(operation string, details string) {
	l.InfoWithContext("DATABASE", "数据库操作: %s - %s", operation, details)
}

// LogNetworkAction 记录网络操作
func (l *Logger) LogNetworkAction(action string, details string) {
	l.InfoWithContext("NETWORK", "网络操作: %s - %s", action, details)
}

// LogUserAction 记录用户操作
func (l *Logger) LogUserAction(userID int64, username string, action string, details string) {
	l.InfoWithContext("USER", "用户 %d (%s) 执行: %s - %s", userID, username, action, details)
}