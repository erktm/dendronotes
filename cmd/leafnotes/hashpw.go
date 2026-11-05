package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"leafnotes/internal/auth"
)

const minPasswordLen = 8

// hashPassword reads a password – hidden and twice when stdin is a terminal,
// otherwise the first line of stdin – and prints its argon2id hash.
func hashPassword(stdin io.Reader, stdout, stderr io.Writer) int {
	pw, err := readPassword(stdin, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if len(pw) < minPasswordLen {
		fmt.Fprintf(stderr, "error: use at least %d characters (a long passphrase is best)\n", minPasswordLen)
		return 1
	}
	hash, err := auth.HashPassword(pw, auth.DefaultParams)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	fmt.Fprintln(stdout, hash)
	return 0
}

func readPassword(stdin io.Reader, stderr io.Writer) (string, error) {
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(stderr, "Password: ")
		first, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(stderr)
		if err != nil {
			return "", err
		}
		fmt.Fprint(stderr, "Repeat password: ")
		second, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(stderr)
		if err != nil {
			return "", err
		}
		if string(first) != string(second) {
			return "", errors.New("the passwords do not match")
		}
		return string(first), nil
	}
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
