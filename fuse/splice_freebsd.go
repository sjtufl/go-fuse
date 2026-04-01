package fuse

import "fmt"

func (s *Server) setSplice() {
	s.canSplice = false
}

func (ms *Server) trySplice(header []byte, req *request, fdData *readResultFd) error {
	return fmt.Errorf("unimplemented")
}

func (ms *Server) trySpliceMulti(req *request, mfd *readResultMultiFd) error {
	return fmt.Errorf("unimplemented")
}
