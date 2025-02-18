package types

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/pkg/errors"
)

type Invoker struct {
	PrintResponse bool
	Client        *http.Client
	GatewayURL    string
	Responses     chan InvokerResponse
}

type InvokerResponse struct {
	Context  context.Context
	Body     *[]byte
	Header   *http.Header
	Status   int
	Error    error
	Topic    string
	Function string
}

func NewInvoker(gatewayURL string, client *http.Client, printResponse bool) *Invoker {
	return &Invoker{
		PrintResponse: printResponse,
		Client:        client,
		GatewayURL:    gatewayURL,
		Responses:     make(chan InvokerResponse),
	}
}

// Invoke triggers a function by accessing the API Gateway
func (i *Invoker) Invoke(topicMap *TopicMap, topic string, message *[]byte) {
	i.InvokeWithContext(context.Background(), topicMap, topic, message, nil)
}

// Invoke triggers a function by accessing the API Gateway while propagating context
func (i *Invoker) InvokeWithContext(ctx context.Context, topicMap *TopicMap, topic string, message *[]byte, headers *map[string]string) {
	if len(*message) == 0 {
		i.Responses <- InvokerResponse{
			Context: ctx,
			Error:   fmt.Errorf("no message to send"),
		}
	}

	matchedFunctions := topicMap.Match(topic)
	for _, matchedFunction := range matchedFunctions {
		log.Printf("Invoke function: %s", matchedFunction)

		gwURL := fmt.Sprintf("%s/%s", i.GatewayURL, matchedFunction)
		reader := bytes.NewReader(*message)

		body, statusCode, header, doErr := invokefunction(ctx, i.Client, gwURL, reader, headers)

		if doErr != nil {
			i.Responses <- InvokerResponse{
				Context: ctx,
				Error:   errors.Wrap(doErr, fmt.Sprintf("unable to invoke %s", matchedFunction)),
			}
			continue
		}

		i.Responses <- InvokerResponse{
			Context:  ctx,
			Body:     body,
			Status:   statusCode,
			Header:   header,
			Function: matchedFunction,
			Topic:    topic,
		}
	}
}

func invokefunction(ctx context.Context, c *http.Client, gwURL string, reader io.Reader, headers *map[string]string) (*[]byte, int, *http.Header, error) {

	httpReq, _ := http.NewRequest(http.MethodPost, gwURL, reader)
	httpReq.WithContext(ctx)

	if httpReq.Body != nil {
		defer httpReq.Body.Close()
	}

	if headers != nil {
    mapOfArrays := make(map[string][]string)
    for k, v := range *headers {
      mapOfArrays[k] = []string{v}
    }
    httpReq.Header = mapOfArrays
  }
	var body *[]byte

	res, doErr := c.Do(httpReq)
	if doErr != nil {
		return nil, http.StatusServiceUnavailable, nil, doErr
	}

	if res.Body != nil {
		defer res.Body.Close()

		bytesOut, readErr := ioutil.ReadAll(res.Body)
		if readErr != nil {
			log.Printf("Error reading body")
			return nil, http.StatusServiceUnavailable, nil, doErr

		}
		body = &bytesOut
	}

	return body, res.StatusCode, &res.Header, doErr
}
