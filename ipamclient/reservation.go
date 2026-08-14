package azureipamclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// internal Models
type reservationSpaceRequest struct {
	Blocks        []string `json:"blocks"`
	Size          *int32   `json:"size"`
	Description   *string  `json:"desc"`
	ReverseSearch bool     `json:"reverse_search"`
	SmallestCidr  bool     `json:"smallest_cidr"`
}
type reservationBlockSizeRequest struct {
	Size          int32   `json:"size"`
	Description   *string `json:"desc"`
	ReverseSearch bool    `json:"reverse_search"`
	SmallestCidr  bool    `json:"smallest_cidr"`
}
type reservationBlockCidrRequest struct {
	Cidr        string  `json:"cidr"`
	Description *string `json:"desc"`
}

const maxRetries = 3

// GetReservations - Returns all existing reservations by space and block
func (c *Client) GetReservations(space, block string, includeSettled bool) (*[]Reservation, error) {
	//prepare request
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/spaces/%s/blocks/%s/reservations?settled=%t", c.HostURL, space, block, includeSettled), nil)
	if err != nil {
		return nil, err
	}
	response, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	//process response
	var reservationsInfo []Reservation
	err = json.Unmarshal(response, &reservationsInfo)
	if err != nil {
		return nil, err
	}

	return &reservationsInfo, nil
}

// GetReservation - Search for a specifc reservation ID iterating spaces and blocks
func (c *Client) FindReservationById(id string) (*Reservation, error) {

	//read all reservations
	spaces, err := c.GetSpaces(false, false)
	if err != nil {
		return nil, err
	}

	//find the reservation by Id, and returns
	for _, space := range *spaces {
		for _, block := range space.Blocks {
			for _, reservation := range block.Reservations {
				if reservation.Id == id {
					return c.GetReservation(space.Name, block.Name, reservation.Id)
				}
			}
		}
	}

	//not found -> Error
	return nil, fmt.Errorf("Reservation not found: %s", id)
}

// GetReservation - Search for a specifc reservation ID iterating spaces and blocks
func (c *Client) GetReservation(space string, block string, id string) (*Reservation, error) {

	//read all reservations
	reservationsInfo, err := c.GetReservations(space, block, true)
	if err != nil {
		return nil, err
	}

	//find the reservation by Id, and returns
	for _, reservation := range *reservationsInfo {
		if reservation.Id == id {
			return &reservation, nil
		}
	}

	//not found -> Error
	return nil, fmt.Errorf("Reservation not found: %s", id)
}

// CreateReservation - Create new reservation
func (c *Client) CreateReservation(space string, blocks []string, description *string, size *int32, specific_cidr *string, reverseSearch bool, smallestCidr bool) (*Reservation, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	//validate params
	if size == nil && specific_cidr == nil {
		return nil, errors.New("at least one of size or specific_cidr must be specified to create a reservation")
	} else if len(blocks) > 1 && specific_cidr != nil {
		return nil, errors.New("specific_cidr is only allowed when only a block is specified in the list")
	}

	//prepare request and body
	var req *http.Request
	var errReq error
	if len(blocks) == 1 {
		//Only one block specified

		if specific_cidr != nil {
			//specific_cidr specified
			request := &reservationBlockCidrRequest{
				Cidr:        *specific_cidr,
				Description: description,
			}
			rb, err := json.Marshal(request)
			if err != nil {
				return nil, err
			}
			req, errReq = http.NewRequest("POST", fmt.Sprintf("%s/api/spaces/%s/blocks/%s/reservations", c.HostURL, space, blocks[0]), strings.NewReader(string(rb)))
			if errReq != nil {
				return nil, err
			}
		} else {
			//reservation by size
			request := &reservationBlockSizeRequest{
				Size:          *size,
				ReverseSearch: reverseSearch,
				SmallestCidr:  smallestCidr,
				Description:   description,
			}
			rb, err := json.Marshal(request)
			if err != nil {
				return nil, err
			}
			req, errReq = http.NewRequest("POST", fmt.Sprintf("%s/api/spaces/%s/blocks/%s/reservations", c.HostURL, space, blocks[0]), strings.NewReader(string(rb)))
			if errReq != nil {
				return nil, err
			}
		}

	} else if len(blocks) > 1 {
		request := &reservationSpaceRequest{
			Size:          size,
			Blocks:        blocks,
			ReverseSearch: reverseSearch,
			SmallestCidr:  smallestCidr,
			Description:   description,
		}
		rb, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		req, errReq = http.NewRequest("POST", fmt.Sprintf("%s/api/spaces/%s/reservations", c.HostURL, space), strings.NewReader(string(rb)))
		if errReq != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("at least one block must be specified")
	}

	//Perform request with retry on 500
	var response []byte
	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			base := time.Duration(attempt) * 2 * time.Second
			jitter := time.Duration(rand.Int63n(int64(time.Second)))
			time.Sleep(base + jitter)
			// Re-build the request body since the body reader is exhausted after the first attempt
			req, err = rebuildReservationRequest(c.HostURL, space, blocks, description, size, specific_cidr, reverseSearch, smallestCidr)
			if err != nil {
				return nil, err
			}
		}
		response, err = c.doRequest(req)
		if err == nil {
			break
		}
		if !isRetryable(err) || attempt == maxRetries {
			return nil, err
		}
	}

	//process response
	reservation := Reservation{}
	err = json.Unmarshal(response, &reservation)
	if err != nil {
		return nil, err
	}

	return &reservation, nil
}

// isRetryable returns true if the error is a transient IPAM API error worth retrying.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return len(msg) >= 10 && (msg[:10] == "status: 500" || msg[:10] == "status: 403")
}

// rebuildReservationRequest re-creates the *http.Request (with a fresh body) for retry attempts.
func rebuildReservationRequest(hostURL, space string, blocks []string, description *string, size *int32, specificCidr *string, reverseSearch, smallestCidr bool) (*http.Request, error) {
	if len(blocks) == 1 {
		if specificCidr != nil {
			rb, err := json.Marshal(&reservationBlockCidrRequest{Cidr: *specificCidr, Description: description})
			if err != nil {
				return nil, err
			}
			return http.NewRequest("POST", fmt.Sprintf("%s/api/spaces/%s/blocks/%s/reservations", hostURL, space, blocks[0]), strings.NewReader(string(rb)))
		}
		rb, err := json.Marshal(&reservationBlockSizeRequest{Size: *size, Description: description, ReverseSearch: reverseSearch, SmallestCidr: smallestCidr})
		if err != nil {
			return nil, err
		}
		return http.NewRequest("POST", fmt.Sprintf("%s/api/spaces/%s/blocks/%s/reservations", hostURL, space, blocks[0]), strings.NewReader(string(rb)))
	}
	rb, err := json.Marshal(&reservationSpaceRequest{Size: size, Blocks: blocks, Description: description, ReverseSearch: reverseSearch, SmallestCidr: smallestCidr})
	if err != nil {
		return nil, err
	}
	return http.NewRequest("POST", fmt.Sprintf("%s/api/spaces/%s/reservations", hostURL, space), strings.NewReader(string(rb)))
}

// DeleteReservation - Deletes a reservation
func (c *Client) DeleteReservation(space, block, id string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	//construct body
	request := [1]string{id}
	rb, err := json.Marshal(request)
	if err != nil {
		return err
	}

	//prepare request
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/api/spaces/%s/blocks/%s/reservations", c.HostURL, space, block), strings.NewReader(string(rb)))
	if err != nil {
		return err
	}

	//Perform request with retry on 500
	var response []byte
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			base := time.Duration(attempt) * 2 * time.Second
			jitter := time.Duration(rand.Int63n(int64(time.Second)))
			time.Sleep(base + jitter)
			req, err = http.NewRequest("DELETE", fmt.Sprintf("%s/api/spaces/%s/blocks/%s/reservations", c.HostURL, space, block), strings.NewReader(string(rb)))
			if err != nil {
				return err
			}
		}
		response, err = c.doRequest(req)
		if err == nil {
			break
		}
		if !isRetryable(err) || attempt == maxRetries {
			return err
		}
	}

	//process response
	if string(response) != "" {
		return errors.New(string(response))
	}

	return nil
}
