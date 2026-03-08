// Package server implements OP's gRPC service — the network facet.
package server

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/organic-programming/go-holons/pkg/transport"
	opv1 "github.com/organic-programming/grace-op/gen/go/op/v1"
	"github.com/organic-programming/grace-op/internal/holons"
	"github.com/organic-programming/grace-op/internal/who"
	"github.com/organic-programming/grace-op/pkg/identity"

	"google.golang.org/grpc"
	grpcReflection "google.golang.org/grpc/reflection"
)

// Server implements the OPService gRPC interface.
type Server struct {
	opv1.UnimplementedOPServiceServer
}

// --- OP-native RPCs ---

// Discover scans for all known holons.
func (s *Server) Discover(ctx context.Context, req *opv1.DiscoverRequest) (*opv1.DiscoverResponse, error) {
	root := req.RootDir
	if root == "" {
		root = "."
	}

	localHolons, err := holons.DiscoverHolons(root)
	if err != nil {
		return nil, err
	}

	entries := make([]*opv1.HolonEntry, 0, len(localHolons))
	for _, h := range localHolons {
		entries = append(entries, &opv1.HolonEntry{
			Identity:     toProto(h.Identity),
			Origin:       h.Origin,
			RelativePath: h.RelativePath,
		})
	}

	pathBinaries := holons.DiscoverInPath()

	return &opv1.DiscoverResponse{
		Entries:      entries,
		PathBinaries: pathBinaries,
	}, nil
}

// Invoke dispatches a command to a holon by name.
func (s *Server) Invoke(ctx context.Context, req *opv1.InvokeRequest) (*opv1.InvokeResponse, error) {
	binary, err := holons.ResolveBinary(req.Holon)
	if err != nil {
		return &opv1.InvokeResponse{
			ExitCode: 1,
			Stderr:   fmt.Sprintf("holon %q not found", req.Holon),
		}, nil
	}

	cmd := exec.CommandContext(ctx, binary, req.Args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := int32(0)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = int32(exitErr.ExitCode())
		} else {
			return nil, fmt.Errorf("failed to run %s: %w", req.Holon, err)
		}
	}

	return &opv1.InvokeResponse{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

// --- Promoted identity RPCs ---

// CreateIdentity creates a new holon identity.
func (s *Server) CreateIdentity(ctx context.Context, req *opv1.CreateIdentityRequest) (*opv1.CreateIdentityResponse, error) {
	return who.Create(req)
}

// ListIdentities lists all known holon identities.
func (s *Server) ListIdentities(ctx context.Context, req *opv1.ListIdentitiesRequest) (*opv1.ListIdentitiesResponse, error) {
	root := "."
	if req != nil && req.GetRootDir() != "" {
		root = req.GetRootDir()
	}
	return who.List(root)
}

// ShowIdentity retrieves a holon's identity by UUID.
func (s *Server) ShowIdentity(ctx context.Context, req *opv1.ShowIdentityRequest) (*opv1.ShowIdentityResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("uuid is required")
	}
	return who.Show(req.GetUuid())
}

// ListenAndServe starts the gRPC server on the given transport URI.
// Supported URIs: tcp://<host>:<port>, unix://<path>, stdio://
func ListenAndServe(listenURI string, reflect bool) error {
	lis, err := transport.Listen(listenURI)
	if err != nil {
		return fmt.Errorf("listen %s: %w", listenURI, err)
	}

	s := grpc.NewServer()
	opv1.RegisterOPServiceServer(s, &Server{})
	if reflect {
		grpcReflection.Register(s)
	}

	mode := "reflection ON"
	if !reflect {
		mode = "reflection OFF"
	}
	log.Printf("OP gRPC server listening on %s (%s)", listenURI, mode)
	return s.Serve(lis)
}

// --- Helpers ---

func toProto(id identity.Identity) *opv1.HolonIdentity {
	return &opv1.HolonIdentity{
		Uuid:         id.UUID,
		GivenName:    id.GivenName,
		FamilyName:   id.FamilyName,
		Motto:        id.Motto,
		Composer:     id.Composer,
		Clade:        cladeToProto(id.Clade),
		Status:       statusToProto(id.Status),
		Born:         id.Born,
		Parents:      id.Parents,
		Reproduction: reproductionToProto(id.Reproduction),
		Aliases:      id.Aliases,
		GeneratedBy:  id.GeneratedBy,
		Lang:         id.Lang,
		ProtoStatus:  statusToProto(id.ProtoStatus),
	}
}

func cladeToProto(value string) opv1.Clade {
	switch lowerTrim(value) {
	case "deterministic/pure":
		return opv1.Clade_DETERMINISTIC_PURE
	case "deterministic/stateful":
		return opv1.Clade_DETERMINISTIC_STATEFUL
	case "deterministic/io_bound":
		return opv1.Clade_DETERMINISTIC_IO_BOUND
	case "probabilistic/generative":
		return opv1.Clade_PROBABILISTIC_GENERATIVE
	case "probabilistic/perceptual":
		return opv1.Clade_PROBABILISTIC_PERCEPTUAL
	case "probabilistic/adaptive":
		return opv1.Clade_PROBABILISTIC_ADAPTIVE
	default:
		return opv1.Clade_CLADE_UNSPECIFIED
	}
}

func statusToProto(value string) opv1.Status {
	switch lowerTrim(value) {
	case "draft":
		return opv1.Status_DRAFT
	case "stable":
		return opv1.Status_STABLE
	case "deprecated":
		return opv1.Status_DEPRECATED
	case "dead":
		return opv1.Status_DEAD
	default:
		return opv1.Status_STATUS_UNSPECIFIED
	}
}

func reproductionToProto(value string) opv1.ReproductionMode {
	switch lowerTrim(value) {
	case "manual":
		return opv1.ReproductionMode_MANUAL
	case "assisted":
		return opv1.ReproductionMode_ASSISTED
	case "automatic":
		return opv1.ReproductionMode_AUTOMATIC
	case "autopoietic":
		return opv1.ReproductionMode_AUTOPOIETIC
	case "bred":
		return opv1.ReproductionMode_BRED
	default:
		return opv1.ReproductionMode_REPRODUCTION_UNSPECIFIED
	}
}

func lowerTrim(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
