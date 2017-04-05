package logging

import (
	"context"
	"log"
)

var silentKey = new(int)

// SetSilent indicates that logs should not actually be omitted for this ctx
func SetSilent(ctx context.Context, val bool) context.Context {
	return context.WithValue(ctx, silentKey, val)
}

// Silent indicates whether this context has been marked as silent (so
// we shouldn't chunder logs)
func Silent(ctx context.Context) bool {
	val, ok := ctx.Value(silentKey).(bool)
	return ok && val
}

// Log shells out to log.Print if Silent is not set.
func Log(c context.Context, v ...interface{}) {
	if c != nil {
		if !Silent(c) {
			log.Print(v...)
		}
	} else {
		log.Print(v...)
	}
}

// Logf shells out to log.Printf if Silent is not set.
func Logf(c context.Context, format string, v ...interface{}) {
	if c != nil {
		if !Silent(c) {
			log.Printf(format, v...)
		}
	} else {
		log.Printf(format, v...)
	}
}

// PadRight right-pads a string.
func PadRight(
	str string,
	pad string,
	lenght int,
) string {
	for {
		str += pad
		if len(str) > lenght {
			return str[0:lenght]
		}
	}
}
