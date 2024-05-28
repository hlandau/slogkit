// Package slogwriter provides a modified version of slog's default text output
// formatter which provides prettier output.
//
// The following changes are implemented:
//
//   - Support for coloured output using ANSI escape codes
//
//   - Support for using a callback function to output log data including record context data
package slogwriter
