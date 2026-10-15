package health

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCheckAvailability(t *testing.T) {

	t.Run("valid/available", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDBPinger := NewMockDbPinger(ctrl)
		mockDBPinger.EXPECT().Ping(gomock.Any()).Return(nil)
		s := NewApplication(mockDBPinger)
		result := s.CheckAvailability(t.Context())
		assert.NoError(t, result.AvailabilityResults[0].Error)
		assert.True(t, result.AvailabilityResults[0].IsAvailable)
	})

	t.Run("invalid/unavailable", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDBPinger := NewMockDbPinger(ctrl)
		mockDBPinger.EXPECT().Ping(gomock.Any()).Return(errors.New("connection failed"))
		s := NewApplication(mockDBPinger)
		result := s.CheckAvailability(t.Context())
		assert.Error(t, result.AvailabilityResults[0].Error)
		assert.False(t, result.AvailabilityResults[0].IsAvailable)
	})

}
