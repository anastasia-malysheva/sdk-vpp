// Copyright (c) 2020-2021 Cisco and/or its affiliates.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memif

import (
	"context"
	"net/url"

	"git.fd.io/govpp.git/api"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	memifMech "github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/memif"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/next"
	"github.com/networkservicemesh/sdk/pkg/networkservice/utils/metadata"
	"github.com/networkservicemesh/sdk/pkg/tools/postpone"

	"github.com/networkservicemesh/sdk-vpp/pkg/tools/ifindex"
)

type memifServer struct {
	vppConn            api.Connection
	directMemifEnabled bool
}

// NewServer provides a NetworkServiceServer chain elements that support the memif Mechanism
func NewServer(vppConn api.Connection, options ...Option) networkservice.NetworkServiceServer {
	m := &memifServer{
		vppConn:            vppConn,
		directMemifEnabled: false,
	}

	for _, opt := range options {
		opt(m)
	}

	return m
}

func (m *memifServer) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (*networkservice.Connection, error) {
	postponeCtxFunc := postpone.ContextWithValues(ctx)

	conn, err := next.Server(ctx).Request(ctx, request)
	if err != nil {
		return nil, err
	}

	if mechanism := memifMech.ToMechanism(conn.GetMechanism()); mechanism != nil {
		// Direct memif if applicable
		if memifSocketAddDel, ok := load(ctx, true); ok && m.directMemifEnabled {
			_, ok := ifindex.Load(ctx, true)
			if ok {
				if err := del(ctx, conn, m.vppConn, true); err != nil {
					if closeErr := m.closeOnFailure(postponeCtxFunc, conn); closeErr != nil {
						err = errors.Wrapf(err, "connection closed with error: %s", closeErr.Error())
					}
					return nil, err
				}
				mechanism.SetSocketFileURL((&url.URL{Scheme: memifMech.SocketFileScheme, Path: memifSocketAddDel.SocketFilename}).String())
				delete(ctx, true)
				ifindex.Delete(ctx, true)
				return conn, nil
			}
		}
	}

	if err := create(ctx, conn, m.vppConn, metadata.IsClient(m)); err != nil {
		if closeErr := m.closeOnFailure(postponeCtxFunc, conn); closeErr != nil {
			err = errors.Wrapf(err, "connection closed with error: %s", closeErr.Error())
		}
		return nil, err
	}
	return conn, nil
}

func (m *memifServer) closeOnFailure(postponeCtxFunc func() (context.Context, context.CancelFunc), conn *networkservice.Connection) error {
	closeCtx, cancelClose := postponeCtxFunc()
	defer cancelClose()

	_, err := m.Close(closeCtx, conn)

	return err
}

func (m *memifServer) Close(ctx context.Context, conn *networkservice.Connection) (*empty.Empty, error) {
	_ = del(ctx, conn, m.vppConn, metadata.IsClient(m))
	return next.Server(ctx).Close(ctx, conn)
}
