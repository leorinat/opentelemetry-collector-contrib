// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package compress // import "github.com/leorinat/opentelemetry-collector-contrib/exporter/awskinesisexporter/internal/compress"

import "io"

type noop struct {
	data io.Writer
}

func (n *noop) Close() error {
	//TODO implement me
	panic("implement me")
}

func NewNoopCompressor() Compressor {
	return &compressor{
		compression: &noop{},
	}
}

func (n *noop) Reset(w io.Writer) {
	n.data = w
}

func (n noop) Write(p []byte) (int, error) {
	return n.data.Write(p)
}

func (n noop) Flush() error {
	return nil
}
