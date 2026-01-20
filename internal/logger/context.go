package logger

import "context"

type loggerKeyType struct{}

func FromContext(ctx context.Context) *Logger {
	return ctx.Value(loggerKeyType{}).(*Logger)
}

func InjectIntoContext(ctx context.Context, log *Logger) context.Context {
	return context.WithValue(ctx, loggerKeyType{}, log)
}
