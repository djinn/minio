/*
 * Minio Cloud Storage, (C) 2016 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"sync"
	"time"

	. "gopkg.in/check.v1"
)

// API suite container.
type TestSuiteXL struct {
	testServer TestServer
	endPoint   string
	accessKey  string
	secretKey  string
}

// Initializing the test suite.
var _ = Suite(&TestSuiteXL{})

// Setting up the test suite.
// Starting the Test server with temporary XL backend.
func (s *TestSuiteXL) SetUpSuite(c *C) {
	s.testServer = StartTestServer(c, "XL")
	s.endPoint = s.testServer.Server.URL
	s.accessKey = s.testServer.AccessKey
	s.secretKey = s.testServer.SecretKey

}

// Called implicitly by "gopkg.in/check.v1" after all tests are run.
func (s *TestSuiteXL) TearDownSuite(c *C) {
	s.testServer.Stop()
}

func (s *TestSuiteXL) TestAuth(c *C) {
	secretID, err := genSecretAccessKey()
	c.Assert(err, IsNil)

	accessID, err := genAccessKeyID()
	c.Assert(err, IsNil)

	c.Assert(len(secretID), Equals, minioSecretID)
	c.Assert(len(accessID), Equals, minioAccessID)
}

// TestBucketPolicy - Inserts the bucket policy and verifies it by fetching the policy back.
// Deletes the policy and verifies the deletion by fetching it back.
func (s *TestSuiteXL) TestBucketPolicy(c *C) {
	// Sample bucket policy.
	bucketPolicyBuf := `{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Action": [
                "s3:GetBucketLocation",
                "s3:ListBucket"
            ],
            "Effect": "Allow",
            "Principal": {
                "AWS": [
                    "*"
                ]
            },
            "Resource": [
                "arn:aws:s3:::%s"
            ]
        },
        {
            "Action": [
                "s3:GetObject"
            ],
            "Effect": "Allow",
            "Principal": {
                "AWS": [
                    "*"
                ]
            },
            "Resource": [
                "arn:aws:s3:::%s/this*"
            ]
        }
    ]
}`
	// generate a random bucket Name.
	bucketName := getRandomBucketName()
	// create the policy statement string with the randomly generated bucket name.
	bucketPolicyStr := fmt.Sprintf(bucketPolicyBuf, bucketName, bucketName)
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the request.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	// assert the http response status code.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// Put a new bucket policy.
	request, err = newTestRequest("PUT", getPutPolicyURL(s.endPoint, bucketName),
		int64(len(bucketPolicyStr)), bytes.NewReader([]byte(bucketPolicyStr)), s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to create bucket.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusNoContent)

	// Fetch the uploaded policy.
	request, err = newTestRequest("GET", getGetPolicyURL(s.endPoint, bucketName), 0, nil,
		s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	bucketPolicyReadBuf, err := ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)
	// Verify if downloaded policy matches with previousy uploaded.
	c.Assert(bytes.Equal([]byte(bucketPolicyStr), bucketPolicyReadBuf), Equals, true)

	// Delete policy.
	request, err = newTestRequest("DELETE", getDeletePolicyURL(s.endPoint, bucketName), 0, nil,
		s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusNoContent)

	// Verify if the policy was indeed deleted.
	request, err = newTestRequest("GET", getGetPolicyURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusNotFound)
}

// TestDeleteBucket - validates DELETE bucket operation.
func (s *TestSuiteXL) TestDeleteBucket(c *C) {
	bucketName := getRandomBucketName()

	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	// assert the response status code.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// construct request to delete the bucket.
	request, err = newTestRequest("DELETE", getDeleteBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// Assert the response status code.
	c.Assert(response.StatusCode, Equals, http.StatusNoContent)
}

// TestDeleteBucketNotEmpty - Validates the operation during an attempt to delete a non-empty bucket.
func (s *TestSuiteXL) TestDeleteBucketNotEmpty(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()

	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the request.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	// assert the response status code.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// generate http request for an object upload.
	// "test-object" is the object name.
	objectName := "test-object"
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the request to complete object upload.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// assert the status code of the response.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// constructing http request to delete the bucket.
	// making an attempt to delete an non-empty bucket.
	// expected to fail.
	request, err = newTestRequest("DELETE", getDeleteBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusConflict)
}

// TestDeleteObject - uploads the object first.
func (s *TestSuiteXL) TestDeleteObject(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the request.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	// assert the http response status code.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	objectName := "prefix/myobject"
	// obtain http request to upload object.
	// object Name contains a prefix.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the http request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// assert the status of http response.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// object name was "prefix/myobject", an attempt to delelte "prefix"
	// Should not delete "prefix/myobject"
	request, err = newTestRequest("DELETE", getDeleteObjectURL(s.endPoint, bucketName, "prefix"),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	client = http.Client{}
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusNoContent)

	// create http request to HEAD on the object.
	// this helps to validate the existence of the bucket.
	request, err = newTestRequest("HEAD", getHeadObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// Assert the HTTP response status code.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// create HTTP request to delete the object.
	request, err = newTestRequest("DELETE", getDeleteObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	client = http.Client{}
	// execute the http request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// assert the http response status code.
	c.Assert(response.StatusCode, Equals, http.StatusNoContent)

	// Delete of non-existent data should return success.
	request, err = newTestRequest("DELETE", getDeleteObjectURL(s.endPoint, bucketName, "prefix/myobject1"),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	client = http.Client{}
	// execute the http request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// assert the http response status.
	c.Assert(response.StatusCode, Equals, http.StatusNoContent)
}

// TestNonExistentBucket - Asserts response for HEAD on non-existent bucket.
func (s *TestSuiteXL) TestNonExistentBucket(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// create request to HEAD on the bucket.
	// HEAD on an bucket helps validate the existence of the bucket.
	request, err := newTestRequest("HEAD", getHEADBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the http request.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	// Assert the response.
	c.Assert(response.StatusCode, Equals, http.StatusNotFound)
}

// TestEmptyObject - Asserts the response for operation on a 0 byte object.
func (s *TestSuiteXL) TestEmptyObject(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the http request.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	// assert the http response status code.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	objectName := "test-object"
	// construct http request for uploading the object.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the upload request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// assert the http response.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// make HTTP request to fetch the object.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the http request to fetch object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// assert the http response status code.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	var buffer bytes.Buffer
	// extract the body of the response.
	responseBody, err := ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)
	// assert the http response body content.
	c.Assert(true, Equals, bytes.Equal(responseBody, buffer.Bytes()))
}

// TestBucket - Asserts the response for HEAD on an existing bucket.
func (s *TestSuiteXL) TestBucket(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// generating HEAD http request on the bucket.
	// this helps verify existence of the bucket.
	request, err = newTestRequest("HEAD", getHEADBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	// execute the request.
	client = http.Client{}
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// assert the response http status code.
	c.Assert(response.StatusCode, Equals, http.StatusOK)
}

// TestGetObject - Tests fetching of a small object after its insertion into the bucket.
func (s *TestSuiteXL) TestGetObject(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	buffer := bytes.NewReader([]byte("hello world"))
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the make bucket http request.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	// assert the response http status code.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	objectName := "testObject"
	// create HTTP request to upload the object.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buffer.Len()), buffer, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to upload the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// assert the HTTP response status code.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// create HTTP request to fetch the object.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the http request to fetch the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// assert the http response status code.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// extract response body content.
	responseBody, err := ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)
	// assert the HTTP response body content with the expected content.
	c.Assert(responseBody, DeepEquals, []byte("hello world"))

}

// TestMultipleObjects - Validates upload and fetching of multiple object into the bucket.
func (s *TestSuiteXL) TestMultipleObjects(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create the bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// constructing HTTP request to fetch a non-existent object.
	// expected to fail, error response asserted for expected error values later.
	objectName := "testObject"
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// Asserting the error response with the expected values.
	verifyError(c, response, "NoSuchKey", "The specified key does not exist.", http.StatusNotFound)

	objectName = "testObject1"
	// content for the object to be uploaded.
	buffer1 := bytes.NewReader([]byte("hello one"))
	// create HTTP request for the object upload.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request for object upload.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// assert the returned values.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// create HTTP request to fetch the object which was uploaded above.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// assert whether 200 OK response status is obtained.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// extract the response body.
	responseBody, err := ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)
	// assert the content body for the expected object data.
	c.Assert(true, Equals, bytes.Equal(responseBody, []byte("hello one")))

	// data for new object to be uploaded.
	buffer2 := bytes.NewReader([]byte("hello two"))
	objectName = "testObject2"
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buffer2.Len()), buffer2, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request for object upload.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// assert the response status code for expected value 200 OK.
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// fetch the object which was uploaded above.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to fetch the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// assert the response status code for expected value 200 OK.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// verify response data
	responseBody, err = ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)
	c.Assert(true, Equals, bytes.Equal(responseBody, []byte("hello two")))

	// data for new object to be uploaded.
	buffer3 := bytes.NewReader([]byte("hello three"))
	objectName = "testObject3"
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buffer3.Len()), buffer3, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// verify the response code with the expected value of 200 OK.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// fetch the object which was uploaded above.
	request, err = newTestRequest("GET", getPutObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// verify object.
	responseBody, err = ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)
	c.Assert(true, Equals, bytes.Equal(responseBody, []byte("hello three")))
}

// TestNotImplemented - Validates response for obtaining policy on an non-existent bucket and object.
func (s *TestSuiteXL) TestNotImplemented(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	request, err := newTestRequest("GET", s.endPoint+"/"+bucketName+"/object?policy",
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusNotImplemented)
}

// TestHeader - Validates the error response for an attempt to fetch non-existent object.
func (s *TestSuiteXL) TestHeader(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// obtain HTTP request to fetch an object from non-existent bucket/object.
	request, err := newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, "testObject"),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	// asserting for the expected error response.
	verifyError(c, response, "NoSuchBucket", "The specified bucket does not exist.", http.StatusNotFound)
}

// TestPutBucket - Validating bucket creation.
func (s *TestSuiteXL) TestPutBucket(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// Block 1: Testing for racey access
	// The assertion is removed from this block since the purpose of this block is to find races
	// The purpose this block is not to check for correctness of functionality
	// Run the test with -race flag to utilize this
	var wg sync.WaitGroup
	for i := 0; i < ConcurrencyLevel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// HTTP request to create the bucket.
			request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
				0, nil, s.accessKey, s.secretKey)
			c.Assert(err, IsNil)

			client := http.Client{}
			response, err := client.Do(request)
			defer response.Body.Close()
		}()
	}
	wg.Wait()

	bucketName = getRandomBucketName()
	//Block 2: testing for correctness of the functionality
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	response.Body.Close()
}

// TestCopyObject - Validates copy object.
// The following is the test flow.
// 1. Create bucket.
// 2. Insert Object.
// 3. Use "X-Amz-Copy-Source" header to copy the previously inserted object.
// 4. Validate the content of copied object.
func (s *TestSuiteXL) TestCopyObject(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// content for the object to be inserted.
	buffer1 := bytes.NewReader([]byte("hello world"))
	objectName := "testObject"
	// create HTTP request for object upload.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	request.Header.Set("Content-Type", "application/json")
	c.Assert(err, IsNil)
	// execute the HTTP request for object upload.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	objectName2 := "testObject2"
	// creating HTTP request for uploading the object.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName2),
		0, nil, s.accessKey, s.secretKey)
	// setting the "X-Amz-Copy-Source" to allow copying the content of
	// previously uploaded object.
	request.Header.Set("X-Amz-Copy-Source", "/"+bucketName+"/"+objectName)
	c.Assert(err, IsNil)
	// execute the HTTP request.
	// the content is expected to have the content of previous disk.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// creating HTTP request to fetch the previously uploaded object.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName2),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// executing the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// validating the response status code.
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// reading the response body.
	// response body is expected to have the copied content of the first uploaded object.
	object, err := ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)
	c.Assert(string(object), Equals, "hello world")
	c.Assert(response.Header.Get("Content-Type"), Equals, "application/json")
}

// TestPutObject -  Tests successful put object request.
func (s *TestSuiteXL) TestPutObject(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// content for new object upload.
	buffer1 := bytes.NewReader([]byte("hello world"))
	objectName := "testObject"
	// creating HTTP request for object upload.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request for object upload.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// fetch the object back and verify its contents.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request to fetch the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	c.Assert(response.ContentLength, Equals, int64(len([]byte("hello world"))))
	var buffer2 bytes.Buffer
	// retrive the contents of response body.
	n, err := io.Copy(&buffer2, response.Body)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, int64(len([]byte("hello world"))))
	// asserted the contents of the fetched object with the expected result.
	c.Assert(true, Equals, bytes.Equal(buffer2.Bytes(), []byte("hello world")))
}

// TestPutObjectLongName - Long Object name strings are created and uploaded, validated for success.
func (s *TestSuiteXL) TestPutObjectLongName(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// content for the object to be uploaded.
	buffer := bytes.NewReader([]byte("hello world"))
	// make long object name.
	longObjName := fmt.Sprintf("%0255d/%0255d/%0255d", 1, 1, 1)
	// create new HTTP request to insert the object.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, longObjName),
		int64(buffer.Len()), buffer, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// make long object name.
	longObjName = fmt.Sprintf("%0256d", 1)
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, longObjName),
		int64(buffer.Len()), buffer, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusNotFound)
}

// TestListBuckets - Make request for listing of all buckets.
// XML response is parsed.
// Its success verifies the format of the response.
func (s *TestSuiteXL) TestListBuckets(c *C) {
	// create HTTP request for listing buckets.
	request, err := newTestRequest("GET", getListBucketURL(s.endPoint),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to list buckets.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	var results ListBucketsResponse
	// parse the list bucket response.
	decoder := xml.NewDecoder(response.Body)
	err = decoder.Decode(&results)
	// validating that the xml-decoding/parsing was successfull.
	c.Assert(err, IsNil)
}

// TestNotBeAbleToCreateObjectInNonexistentBucket - Validates the error response
// on an attempt to upload an object into a non-existent bucket.
func (s *TestSuiteXL) TestNotBeAbleToCreateObjectInNonexistentBucket(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// content of the object to be uploaded.
	buffer1 := bytes.NewReader([]byte("hello world"))

	// preparing for upload by generating the upload URL.
	objectName := "test-object"
	request, err := newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// Execute the HTTP request.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	// Assert the response error message.
	verifyError(c, response, "NoSuchBucket", "The specified bucket does not exist.", http.StatusNotFound)
}

// TestGetOnObject - Asserts properties for GET on an object.
// GET requests on an object retrieves the object from server.
// Tests behaviour when If-Match/If-None-Match headers are set on the request.
func (s *TestSuiteXL) TestGetOnObject(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// make HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	buffer1 := bytes.NewReader([]byte("hello world"))
	request, err = newTestRequest("PUT", s.endPoint+"/"+bucketName+"/object1",
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// GetObject with If-Match sending correct etag in request headers
	// is expected to return the object
	md5Writer := md5.New()
	md5Writer.Write([]byte("hello world"))
	etag := hex.EncodeToString(md5Writer.Sum(nil))
	request, err = newTestRequest("GET", s.endPoint+"/"+bucketName+"/object1",
		0, nil, s.accessKey, s.secretKey)
	request.Header.Set("If-Match", etag)
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	var body []byte
	body, err = ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)
	c.Assert(string(body), Equals, "hello world")

	// GetObject with If-Match sending mismatching etag in request headers
	// is expected to return an error response with ErrPreconditionFailed.
	request, err = newTestRequest("GET", s.endPoint+"/"+bucketName+"/object1",
		0, nil, s.accessKey, s.secretKey)
	request.Header.Set("If-Match", etag[1:])
	response, err = client.Do(request)
	verifyError(c, response, "PreconditionFailed", "At least one of the preconditions you specified did not hold.", http.StatusPreconditionFailed)

	// GetObject with If-None-Match sending mismatching etag in request headers
	// is expected to return the object.
	request, err = newTestRequest("GET", s.endPoint+"/"+bucketName+"/object1",
		0, nil, s.accessKey, s.secretKey)
	request.Header.Set("If-None-Match", etag[1:])
	response, err = client.Do(request)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	body, err = ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)
	c.Assert(string(body), Equals, "hello world")

	// GetObject with If-None-Match sending matching etag in request headers
	// is expected to return (304) Not-Modified.
	request, err = newTestRequest("GET", s.endPoint+"/"+bucketName+"/object1",
		0, nil, s.accessKey, s.secretKey)
	request.Header.Set("If-None-Match", etag)
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusNotModified)
}

// TestHeadOnObjectLastModified - Asserts response for HEAD on an object.
// HEAD requests on an object validates the existence of the object.
// The responses for fetching the object when If-Modified-Since
// and If-Unmodified-Since headers set are validated.
// If-Modified-Since - Return the object only if it has been modified since the specified time, else return a 304 (not modified).
// If-Unmodified-Since - Return the object only if it has not been modified since the specified time, else return a 412 (precondition failed).
func (s *TestSuiteXL) TestHeadOnObjectLastModified(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// preparing for object upload.
	objectName := "test-object"
	// content for the object to be uploaded.
	buffer1 := bytes.NewReader([]byte("hello world"))
	// obtaining URL for uploading the object.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	// executing the HTTP request to download the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// make HTTP request to obtain object info.
	request, err = newTestRequest("HEAD", getHeadObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// verify the status of the HTTP response.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// retrive the info of last modification time of the object from the response header.
	lastModified := response.Header.Get("Last-Modified")
	// Parse it into time.Time structure.
	t, err := time.Parse(http.TimeFormat, lastModified)
	c.Assert(err, IsNil)

	// make HTTP request to obtain object info.
	// But this time set the "If-Modified-Since" header to be a minute more than the actual
	// last modified time of the object.
	request, err = newTestRequest("HEAD", getHeadObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	request.Header.Set("If-Modified-Since", t.Add(1*time.Minute).UTC().Format(http.TimeFormat))
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// Since the "If-Modified-Since" header was ahead in time compared to the actual modified time of the object
	// expecting the response status to be http.StatusNotModified.
	c.Assert(response.StatusCode, Equals, http.StatusNotModified)

	// Again, obtain the object info.
	// This time setting "If-Unmodified-Since" to a time after the object is modified.
	// As documented above, expecting http.StatusPreconditionFailed.
	request, err = newTestRequest("HEAD", getHeadObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	request.Header.Set("If-Unmodified-Since", t.Add(-1*time.Minute).UTC().Format(http.TimeFormat))
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusPreconditionFailed)
}

// TestHeadOnBucket - Validates response for HEAD on the bucket.
// HEAD request on the bucket validates the existence of the bucket.
func (s *TestSuiteXL) TestHeadOnBucket(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getHEADBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// make HEAD request on the bucket.
	request, err = newTestRequest("HEAD", getHEADBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// Asserting the response status for expected value of http.StatusOK.
	c.Assert(response.StatusCode, Equals, http.StatusOK)
}

// TestContentTypePersists - Object upload with different Content-type is first done.
// And then a HEAD and GET request on these objects are done to validate if the same Content-Type set during upload persists.
func (s *TestSuiteXL) TestContentTypePersists(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// Uploading a new object with Content-Type  "application/zip".
	// content for the object to be uploaded.
	buffer1 := bytes.NewReader([]byte("hello world"))
	objectName := "test-1-object"
	// constructing HTTP request for object upload.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// setting the Content-Type header to be application/zip.
	// After object upload a validation will be done to see if the Content-Type set persists.
	request.Header.Set("Content-Type", "application/zip")

	client = http.Client{}
	// execute the HTTP request for object upload.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// Fetching the object info using HEAD request for the object which was uploaded above.
	request, err = newTestRequest("HEAD", getHeadObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	// Execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// Verify if the Content-Type header is set during the object persists.
	c.Assert(response.Header.Get("Content-Type"), Equals, "application/zip")

	// Fetching the object itself and then verify the Content-Type header.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// Execute the HTTP to fetch the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// Verify if the Content-Type header is set during the object persists.
	c.Assert(response.Header.Get("Content-Type"), Equals, "application/zip")

	// Uploading a new object with Content-Type  "application/json".
	objectName = "test-2-object"
	buffer2 := bytes.NewReader([]byte("hello world"))
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buffer2.Len()), buffer2, s.accessKey, s.secretKey)
	// deleting the old header value.
	delete(request.Header, "Content-Type")
	// setting the request header to be application/json.
	request.Header.Add("Content-Type", "application/json")
	c.Assert(err, IsNil)

	// Execute the HTTP request to upload the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// Obtain the info of the object which was uploaded above using HEAD request.
	request, err = newTestRequest("HEAD", getHeadObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// Execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// Assert if the content-type header set during the object upload persists.
	c.Assert(response.Header.Get("Content-Type"), Equals, "application/json")

	// Fetch the object and assert whether the Content-Type header persists.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	// Execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// Assert if the content-type header set during the object upload persists.
	c.Assert(response.Header.Get("Content-Type"), Equals, "application/json")
}

// TestPartialContent - Validating for GetObject with partial content request.
// By setting the Range header, A request to send specific bytes range of data from an
// already uploaded object can be done.
func (s *TestSuiteXL) TestPartialContent(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP Request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create the bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// content for the object to be uploaded.
	buffer1 := bytes.NewReader([]byte("Hello World"))
	objectName := "test-object"
	// make HTTP request to upload the object.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// Execute the HTTP request for object upload.
	response, err = client.Do(request)
	// verify that the upload was successfull.
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// Set of cases which will be used to create Get Object requests with specific content range.
	var table = []struct {
		byteRange      string
		expectedString string
	}{
		// Request bytes 6-7 of the uploaded object.
		// Since the object content was "Hello World", expecting "Wo" in the response.
		{"6-7", "Wo"},
		// Request for first 6 bytes of object content.
		{"6-", "World"},
		// Request for last 7 bytes of the object content.
		{"-7", "o World"},
	}
	for _, t := range table {
		// make HTTP request to fetch the object.
		request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
			0, nil, s.accessKey, s.secretKey)
		c.Assert(err, IsNil)
		// Set the Range header with range of bytes data of the uploaded object to be fetched.
		request.Header.Add("Range", "bytes="+t.byteRange)

		client = http.Client{}
		// execute the HTTP request.
		response, err = client.Do(request)
		c.Assert(err, IsNil)
		// since the complete object is not fetched,
		// http.StatusPartialContent is expected to be the response status.
		c.Assert(response.StatusCode, Equals, http.StatusPartialContent)
		// parse the response body to obtain the partial content requested for.
		partialObject, err := ioutil.ReadAll(response.Body)
		c.Assert(err, IsNil)
		// asserting the obtained partial content with the expected data.
		c.Assert(string(partialObject), Equals, t.expectedString)
	}
}

// TestListObjectsHandlerErrors - Setting invalid parameters to List Objects
// and then asserting the error response with the expected one.
func (s *TestSuiteXL) TestListObjectsHandlerErrors(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// create HTTP request with invalid value of max-keys parameter.
	// max-keys is set to -2.
	request, err = newTestRequest("GET", getListObjectsURL(s.endPoint, bucketName, "-2"),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	client = http.Client{}
	// execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// validating the error response.
	verifyError(c, response, "InvalidArgument", "Argument maxKeys must be an integer between 0 and 2147483647.", http.StatusBadRequest)
}

// TestPutBucketErrors - request for non valid bucket operation
// and validate it with expected error result.
func (s *TestSuiteXL) TestPutBucketErrors(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// generating a HTTP request to create bucket.
	// using invalid bucket name.
	request, err := newTestRequest("PUT", s.endPoint+"/putbucket-.",
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	// expected to fail with error message "InvalidBucketName".
	verifyError(c, response, "InvalidBucketName", "The specified bucket is not valid.", http.StatusBadRequest)
	// HTTP request to create the bucket.
	request, err = newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to create bucket.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// make HTTP request to create the same bucket again.
	// expected to fail with error message "BucketAlreadyOwnedByYou".
	request, err = newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	response, err = client.Do(request)
	c.Assert(err, IsNil)
	verifyError(c, response, "BucketAlreadyOwnedByYou", "Your previous request to create the named bucket succeeded and you already own it.",
		http.StatusConflict)

	// request for ACL.
	// Since Minio server doesn't support ACL's the request is expected to fail with  "NotImplemented" error message.
	request, err = newTestRequest("PUT", s.endPoint+"/"+bucketName+"?acl",
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	response, err = client.Do(request)
	c.Assert(err, IsNil)
	verifyError(c, response, "NotImplemented", "A header you provided implies functionality that is not implemented.", http.StatusNotImplemented)
}

// TestGetObjectLarge10MiB - Tests validate fetching of an object of size 10MB.
func (s *TestSuiteXL) TestGetObjectLarge10MiB(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// form HTTP reqest to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create the bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	var buffer bytes.Buffer
	line := `1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,123"`
	// Create 10MiB content where each line contains 1024 characters.
	for i := 0; i < 10*1024; i++ {
		buffer.WriteString(fmt.Sprintf("[%05d] %s\n", i, line))
	}
	putContent := buffer.String()

	buf := bytes.NewReader([]byte(putContent))

	objectName := "test-big-object"
	// create HTTP request for object upload.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buf.Len()), buf, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// Assert the status code to verify successful upload.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// prepare HTTP requests to download the object.
	request, err = newTestRequest("GET", getPutObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to download the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// extract the content from response body.
	getContent, err := ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)

	// Compare putContent and getContent.
	c.Assert(string(getContent), Equals, putContent)
}

// TestGetObjectLarge11MiB - Tests validate fetching of an object of size 10MB.
func (s *TestSuiteXL) TestGetObjectLarge11MiB(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	var buffer bytes.Buffer
	line := `1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,123`
	// Create 11MiB content where each line contains 1024 characters.
	for i := 0; i < 11*1024; i++ {
		buffer.WriteString(fmt.Sprintf("[%05d] %s\n", i, line))
	}
	putMD5 := sumMD5(buffer.Bytes())

	objectName := "test-11Mb-object"
	// Put object
	buf := bytes.NewReader(buffer.Bytes())
	// create HTTP request foe object upload.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buf.Len()), buf, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request for object upload.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// create HTTP request to download the object.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// fetch the content from response body.
	getContent, err := ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)

	// Get md5Sum of the response content.
	getMD5 := sumMD5(getContent)

	// Compare putContent and getContent.
	c.Assert(hex.EncodeToString(putMD5), Equals, hex.EncodeToString(getMD5))
}

// TestGetPartialObjectMisAligned - tests get object partially mis-aligned.
// create a large buffer of mis-aligned data and upload it.
// then make partial range requests to while fetching it back and assert the response content.
func (s *TestSuiteXL) TestGetPartialObjectMisAligned(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.testServer.AccessKey, s.testServer.SecretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create the bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	var buffer bytes.Buffer
	line := `1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,123`

	rand.Seed(time.Now().UTC().UnixNano())
	// Create a misalgined data.
	for i := 0; i < 13*rand.Intn(1<<16); i++ {
		buffer.WriteString(fmt.Sprintf("[%05d] %s\n", i, line[:rand.Intn(1<<8)]))
	}
	putContent := buffer.String()
	buf := bytes.NewReader([]byte(putContent))

	objectName := "test-big-file"
	// HTTP request to upload the object.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buf.Len()), buf, s.testServer.AccessKey, s.testServer.SecretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to upload the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// test Cases containing data to make partial range requests.
	// also has expected response data.
	var testCases = []struct {
		byteRange      string
		expectedString string
	}{
		// request for byte range 10-11.
		// expecting the result to contain only putContent[10:12] bytes.
		{"10-11", putContent[10:12]},
		// request for object data after the first byte.
		{"1-", putContent[1:]},
		// request for object data after the first byte.
		{"6-", putContent[6:]},
		// request for last 2 bytes of th object.
		{"-2", putContent[len(putContent)-2:]},
		// request for last 7 bytes of the object.
		{"-7", putContent[len(putContent)-7:]},
	}
	for _, t := range testCases {
		// HTTP request to download the object.
		request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
			0, nil, s.testServer.AccessKey, s.testServer.SecretKey)
		c.Assert(err, IsNil)
		// Get partial content based on the byte range set.
		request.Header.Add("Range", "bytes="+t.byteRange)

		client = http.Client{}
		// execute the HTTP request.
		response, err = client.Do(request)
		c.Assert(err, IsNil)
		// Since only part of the object is requested, expecting response status to be http.StatusPartialContent .
		c.Assert(response.StatusCode, Equals, http.StatusPartialContent)
		// parse the HTTP response body.
		getContent, err := ioutil.ReadAll(response.Body)
		c.Assert(err, IsNil)

		// Compare putContent and getContent.
		c.Assert(string(getContent), Equals, t.expectedString)
	}
}

// TestGetPartialObjectLarge11MiB - Test validates partial content request for a 11MiB object.
func (s *TestSuiteXL) TestGetPartialObjectLarge11MiB(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create the bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	var buffer bytes.Buffer
	line := `234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,123`
	// Create 11MiB content where each line contains 1024
	// characters.
	for i := 0; i < 11*1024; i++ {
		buffer.WriteString(fmt.Sprintf("[%05d] %s\n", i, line))
	}
	putContent := buffer.String()

	objectName := "test-large-11Mb-object"

	buf := bytes.NewReader([]byte(putContent))
	// HTTP request to upload the object.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buf.Len()), buf, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to upload the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// HTTP request to download the object.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// This range spans into first two blocks.
	request.Header.Add("Range", "bytes=10485750-10485769")

	client = http.Client{}
	// execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// Since only part of the object is requested, expecting response status to be http.StatusPartialContent .
	c.Assert(response.StatusCode, Equals, http.StatusPartialContent)
	// read the downloaded content from the response body.
	getContent, err := ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)

	// Compare putContent and getContent.
	c.Assert(string(getContent), Equals, putContent[10485750:10485770])
}

// TestGetPartialObjectLarge11MiB - Test validates partial content request for a 10MiB object.
func (s *TestSuiteXL) TestGetPartialObjectLarge10MiB(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	// expecting the error to be nil.
	c.Assert(err, IsNil)
	// expecting the HTTP response status code to 200 OK.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	var buffer bytes.Buffer
	line := `1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,1234567890,
	1234567890,1234567890,1234567890,123`
	// Create 10MiB content where each line contains 1024 characters.
	for i := 0; i < 10*1024; i++ {
		buffer.WriteString(fmt.Sprintf("[%05d] %s\n", i, line))
	}

	putContent := buffer.String()
	buf := bytes.NewReader([]byte(putContent))

	objectName := "test-big-10Mb-file"
	// HTTP request to upload the object.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buf.Len()), buf, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to upload the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// verify whether upload was successfull.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// HTTP request to download the object.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// Get partial content based on the byte range set.
	request.Header.Add("Range", "bytes=2048-2058")

	client = http.Client{}
	// execute the HTTP request to download the partila content.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// Since only part of the object is requested, expecting response status to be http.StatusPartialContent .
	c.Assert(response.StatusCode, Equals, http.StatusPartialContent)
	// read the downloaded content from the response body.
	getContent, err := ioutil.ReadAll(response.Body)
	c.Assert(err, IsNil)

	// Compare putContent and getContent.
	c.Assert(string(getContent), Equals, putContent[2048:2059])
}

// TestGetObjectErrors - Tests validate error response for invalid object operations.
func (s *TestSuiteXL) TestGetObjectErrors(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()

	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	objectName := "test-non-exitent-object"
	// HTTP request to download the object.
	// Since the specified object doesn't exist in the given bucket,
	// expected to fail with error message "NoSuchKey"
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	verifyError(c, response, "NoSuchKey", "The specified key does not exist.", http.StatusNotFound)

	// request to download an object, but an invalid bucket name is set.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, "/getobjecterrors-.", objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	// execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// expected to fail with "InvalidBucketName".
	verifyError(c, response, "InvalidBucketName", "The specified bucket is not valid.", http.StatusBadRequest)
}

// TestGetObjectRangeErrors - Validate error response when object is fetched with incorrect byte range value.
func (s *TestSuiteXL) TestGetObjectRangeErrors(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// content for the object to be uploaded.
	buffer1 := bytes.NewReader([]byte("Hello World"))

	objectName := "test-object"
	// HTTP request to upload the object.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to upload the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// verify whether upload was successfull.
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// HTTP request to download the object.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	// invalid byte range set.
	request.Header.Add("Range", "bytes=7-6")
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// expected to fail with "InvalidRange" error message.
	verifyError(c, response, "InvalidRange", "The requested range cannot be satisfied.", http.StatusRequestedRangeNotSatisfiable)
}

// TestObjectMultipartAbort - Test validates abortion of a multipart upload after uploading 2 parts.
func (s *TestSuiteXL) TestObjectMultipartAbort(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	objectName := "test-multipart-object"
	// construct HTTP request to initiate a NewMultipart upload.
	request, err = newTestRequest("POST", getNewMultipartURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	// execute the HTTP request initiating the new multipart upload.
	response, err = client.Do(request)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// parse the response body and obtain the new upload ID.
	decoder := xml.NewDecoder(response.Body)
	newResponse := &InitiateMultipartUploadResponse{}

	err = decoder.Decode(newResponse)
	c.Assert(err, IsNil)
	c.Assert(len(newResponse.UploadID) > 0, Equals, true)
	// uploadID to be used for rest of the multipart operations on the object.
	uploadID := newResponse.UploadID

	// content for the part to be uploaded.
	buffer1 := bytes.NewReader([]byte("hello world"))
	// HTTP request for the part to be uploaded.
	request, err = newTestRequest("PUT", getPartUploadURL(s.endPoint, bucketName, objectName, uploadID, "1"),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request to upload the first part.
	response1, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response1.StatusCode, Equals, http.StatusOK)

	// content for the second part to be uploaded.
	buffer2 := bytes.NewReader([]byte("hello world"))
	// HTTP request for the second part to be uploaded.
	request, err = newTestRequest("PUT", getPartUploadURL(s.endPoint, bucketName, objectName, uploadID, "2"),
		int64(buffer2.Len()), buffer2, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request to upload the second part.
	response2, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response2.StatusCode, Equals, http.StatusOK)
	// HTTP request for aborting the multipart upload.
	request, err = newTestRequest("DELETE", getAbortMultipartUploadURL(s.endPoint, bucketName, objectName, uploadID),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request to abort the multipart upload.
	response3, err := client.Do(request)
	c.Assert(err, IsNil)
	// expecting the response status code to be http.StatusNoContent.
	// The assertion validates the success of Abort Multipart operation.
	c.Assert(response3.StatusCode, Equals, http.StatusNoContent)
}

// TestBucketMultipartList - Initiates a NewMultipart upload, uploads parts and validates listing of the parts.
func (s *TestSuiteXL) TestBucketMultipartList(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName), 0,
		nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, 200)

	objectName := "test-multipart-object"
	// construct HTTP request to initiate a NewMultipart upload.
	request, err = newTestRequest("POST", getNewMultipartURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request initiating the new multipart upload.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// expecting the response status code to be http.StatusOK(200 OK) .
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// parse the response body and obtain the new upload ID.
	decoder := xml.NewDecoder(response.Body)
	newResponse := &InitiateMultipartUploadResponse{}

	err = decoder.Decode(newResponse)
	c.Assert(err, IsNil)
	c.Assert(len(newResponse.UploadID) > 0, Equals, true)
	// uploadID to be used for rest of the multipart operations on the object.
	uploadID := newResponse.UploadID

	// content for the part to be uploaded.
	buffer1 := bytes.NewReader([]byte("hello world"))
	// HTTP request for the part to be uploaded.
	request, err = newTestRequest("PUT", getPartUploadURL(s.endPoint, bucketName, objectName, uploadID, "1"),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request to upload the first part.
	response1, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response1.StatusCode, Equals, http.StatusOK)

	// content for the second part to be uploaded.
	buffer2 := bytes.NewReader([]byte("hello world"))
	// HTTP request for the second part to be uploaded.
	request, err = newTestRequest("PUT", getPartUploadURL(s.endPoint, bucketName, objectName, uploadID, "2"),
		int64(buffer2.Len()), buffer2, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request to upload the second part.
	response2, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response2.StatusCode, Equals, http.StatusOK)

	// HTTP request to ListMultipart Uploads.
	request, err = newTestRequest("GET", getListMultipartURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request.
	response3, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response3.StatusCode, Equals, http.StatusOK)

	// The reason to duplicate this structure here is to verify if the
	// unmarshalling works from a client perspective, specifically
	// while unmarshalling time.Time type for 'Initiated' field.
	// time.Time does not honor xml marshaler, it means that we need
	// to encode/format it before giving it to xml marshalling.

	// This below check adds client side verification to see if its
	// truly parseable.

	// listMultipartUploadsResponse - format for list multipart uploads response.
	type listMultipartUploadsResponse struct {
		XMLName xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListMultipartUploadsResult" json:"-"`

		Bucket             string
		KeyMarker          string
		UploadIDMarker     string `xml:"UploadIdMarker"`
		NextKeyMarker      string
		NextUploadIDMarker string `xml:"NextUploadIdMarker"`
		EncodingType       string
		MaxUploads         int
		IsTruncated        bool
		// All the in progress multipart uploads.
		Uploads []struct {
			Key          string
			UploadID     string `xml:"UploadId"`
			Initiator    Initiator
			Owner        Owner
			StorageClass string
			Initiated    time.Time // Keep this native to be able to parse properly.
		}
		Prefix         string
		Delimiter      string
		CommonPrefixes []CommonPrefix
	}

	// parse the response body.
	decoder = xml.NewDecoder(response3.Body)
	newResponse3 := &listMultipartUploadsResponse{}
	err = decoder.Decode(newResponse3)
	c.Assert(err, IsNil)
	// Assert the bucket name in the response with the expected bucketName.
	c.Assert(newResponse3.Bucket, Equals, bucketName)
	// Assert the bucket name in the response with the expected bucketName.
	c.Assert(newResponse3.IsTruncated, Equals, false)
}

// TestMakeBucketLocation - tests make bucket location header response.
func (s *TestSuiteXL) TestMakeBucketLocation(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, 200)
	// Validate location header value equals proper bucket name.
	c.Assert(response.Header.Get("Location"), Equals, "/"+bucketName)
}

// TestValidateObjectMultipartUploadID - Test Initiates a new multipart upload and validates the uploadID.
func (s *TestSuiteXL) TestValidateObjectMultipartUploadID(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, 200)

	objectName := "directory1/directory2/object"
	// construct HTTP request to initiate a NewMultipart upload.
	request, err = newTestRequest("POST", getNewMultipartURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request initiating the new multipart upload.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// parse the response body and obtain the new upload ID.
	decoder := xml.NewDecoder(response.Body)
	newResponse := &InitiateMultipartUploadResponse{}
	err = decoder.Decode(newResponse)
	// expecting the decoding error to be nil.
	c.Assert(err, IsNil)
	// Verifying for Upload ID value to be greater than 0.
	c.Assert(len(newResponse.UploadID) > 0, Equals, true)
}

// TestObjectMultipartListError - Initiates a NewMultipart upload, uploads parts and validates
// error response for an incorrect max-parts parameter .
func (s *TestSuiteXL) TestObjectMultipartListError(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, 200)

	objectName := "test-multipart-object"
	// construct HTTP request to initiate a NewMultipart upload.
	request, err = newTestRequest("POST", getNewMultipartURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request initiating the new multipart upload.
	response, err = client.Do(request)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// parse the response body and obtain the new upload ID.
	decoder := xml.NewDecoder(response.Body)
	newResponse := &InitiateMultipartUploadResponse{}

	err = decoder.Decode(newResponse)
	c.Assert(err, IsNil)
	c.Assert(len(newResponse.UploadID) > 0, Equals, true)
	// uploadID to be used for rest of the multipart operations on the object.
	uploadID := newResponse.UploadID

	// content for the part to be uploaded.
	buffer1 := bytes.NewReader([]byte("hello world"))
	// HTTP request for the part to be uploaded.
	request, err = newTestRequest("PUT", getPartUploadURL(s.endPoint, bucketName, objectName, uploadID, "1"),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request to upload the first part.
	response1, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response1.StatusCode, Equals, http.StatusOK)

	// content for the second part to be uploaded.
	buffer2 := bytes.NewReader([]byte("hello world"))
	// HTTP request for the second part to be uploaded.
	request, err = newTestRequest("PUT", getPartUploadURL(s.endPoint, bucketName, objectName, uploadID, "2"),
		int64(buffer2.Len()), buffer2, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	// execute the HTTP request to upload the second part.
	response2, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response2.StatusCode, Equals, http.StatusOK)
	// HTTP request to ListMultipart Uploads.
	// max-keys is set to invalid value of -2.
	request, err = newTestRequest("GET", getListMultipartURLWithParams(s.endPoint, bucketName, objectName, uploadID, "-2"),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request.
	response4, err := client.Do(request)
	c.Assert(err, IsNil)
	// Since max-keys parameter in the ListMultipart request set to invalid value of -2,
	// its expected to fail with error message "InvalidArgument".
	verifyError(c, response4, "InvalidArgument", "Argument maxParts must be an integer between 1 and 10000.", http.StatusBadRequest)
}

// TestMultipartErrorEntityTooSmall - initiates a new multipart upload,
// uploads 2 parts of size less than 5MB, upon complete multipart upload
// validates EntityTooSmall error returned by the operation.
func (s *TestSuiteXL) TestMultipartErrorEntityTooSmall(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, 200)

	objectName := "test-multipart-object"
	// construct HTTP request to initiate a NewMultipart upload.
	request, err = newTestRequest("POST", getNewMultipartURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request initiating the new multipart upload.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// expecting the response status code to be http.StatusOK(200 OK).
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// parse the response body and obtain the new upload ID.
	decoder := xml.NewDecoder(response.Body)
	newResponse := &InitiateMultipartUploadResponse{}

	err = decoder.Decode(newResponse)
	c.Assert(err, IsNil)
	c.Assert(len(newResponse.UploadID) > 0, Equals, true)
	// uploadID to be used for rest of the multipart operations on the object.
	uploadID := newResponse.UploadID

	// content for the part to be uploaded.
	// Create a byte array of 4MB.
	data := bytes.Repeat([]byte("0123456789abcdef"), 4*1024*1024/16)
	// calculate md5Sum of the data.
	hasher := md5.New()
	hasher.Write(data)
	md5Sum := hasher.Sum(nil)

	buffer1 := bytes.NewReader(data)
	// HTTP request for the part to be uploaded.
	request, err = newTestRequest("PUT", getPartUploadURL(s.endPoint, bucketName, objectName, uploadID, "1"),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	// set the Content-Md5 header to the base64 encoding the md5Sum of the content.
	request.Header.Set("Content-Md5", base64.StdEncoding.EncodeToString(md5Sum))
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to upload the first part.
	response1, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response1.StatusCode, Equals, http.StatusOK)

	// content for the second part to be uploaded will be same as first part.
	hasher = md5.New()
	hasher.Write(data)
	// calculate md5Sum of the data.
	md5Sum = hasher.Sum(nil)

	buffer2 := bytes.NewReader(data)
	// HTTP request for the second part to be uploaded.
	request, err = newTestRequest("PUT", getPartUploadURL(s.endPoint, bucketName, objectName, uploadID, "2"),
		int64(buffer2.Len()), buffer2, s.accessKey, s.secretKey)
	// set the Content-Md5 header to the base64 encoding the md5Sum of the content.
	request.Header.Set("Content-Md5", base64.StdEncoding.EncodeToString(md5Sum))
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to upload the second part.
	response2, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response2.StatusCode, Equals, http.StatusOK)

	// Complete multipart upload
	completeUploads := &completeMultipartUpload{
		Parts: []completePart{
			{
				PartNumber: 1,
				ETag:       response1.Header.Get("ETag"),
			},
			{
				PartNumber: 2,
				ETag:       response2.Header.Get("ETag"),
			},
		},
	}

	completeBytes, err := xml.Marshal(completeUploads)
	c.Assert(err, IsNil)
	// Indicating that all parts are uploaded and initiating completeMultipartUpload.
	request, err = newTestRequest("POST", getCompleteMultipartUploadURL(s.endPoint, bucketName, objectName, uploadID), int64(len(completeBytes)), bytes.NewReader(completeBytes), s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	// Execute the complete multipart request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// verify whether complete multipart was successfull.
	verifyError(c, response, "EntityTooSmall", "Your proposed upload is smaller than the minimum allowed object size.", http.StatusOK)
}

// TestObjectMultipart - Initiates a NewMultipart upload, uploads 2 parts,
// completes the multipart upload and validates the status of the operation.
func (s *TestSuiteXL) TestObjectMultipart(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, 200)

	objectName := "test-multipart-object"
	// construct HTTP request to initiate a NewMultipart upload.
	request, err = newTestRequest("POST", getNewMultipartURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request initiating the new multipart upload.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// expecting the response status code to be http.StatusOK(200 OK).
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// parse the response body and obtain the new upload ID.
	decoder := xml.NewDecoder(response.Body)
	newResponse := &InitiateMultipartUploadResponse{}

	err = decoder.Decode(newResponse)
	c.Assert(err, IsNil)
	c.Assert(len(newResponse.UploadID) > 0, Equals, true)
	// uploadID to be used for rest of the multipart operations on the object.
	uploadID := newResponse.UploadID

	// content for the part to be uploaded.
	// Create a byte array of 5MB.
	data := bytes.Repeat([]byte("0123456789abcdef"), 5*1024*1024/16)
	// calculate md5Sum of the data.
	hasher := md5.New()
	hasher.Write(data)
	md5Sum := hasher.Sum(nil)

	buffer1 := bytes.NewReader(data)
	// HTTP request for the part to be uploaded.
	request, err = newTestRequest("PUT", getPartUploadURL(s.endPoint, bucketName, objectName, uploadID, "1"),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	// set the Content-Md5 header to the base64 encoding the md5Sum of the content.
	request.Header.Set("Content-Md5", base64.StdEncoding.EncodeToString(md5Sum))
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to upload the first part.
	response1, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response1.StatusCode, Equals, http.StatusOK)

	// content for the second part to be uploaded.
	// Create a byte array of 1 byte.
	data = []byte("0")

	hasher = md5.New()
	hasher.Write(data)
	// calculate md5Sum of the data.
	md5Sum = hasher.Sum(nil)

	buffer2 := bytes.NewReader(data)
	// HTTP request for the second part to be uploaded.
	request, err = newTestRequest("PUT", getPartUploadURL(s.endPoint, bucketName, objectName, uploadID, "2"),
		int64(buffer2.Len()), buffer2, s.accessKey, s.secretKey)
	// set the Content-Md5 header to the base64 encoding the md5Sum of the content.
	request.Header.Set("Content-Md5", base64.StdEncoding.EncodeToString(md5Sum))
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to upload the second part.
	response2, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response2.StatusCode, Equals, http.StatusOK)

	// Complete multipart upload
	completeUploads := &completeMultipartUpload{
		Parts: []completePart{
			{
				PartNumber: 1,
				ETag:       response1.Header.Get("ETag"),
			},
			{
				PartNumber: 2,
				ETag:       response2.Header.Get("ETag"),
			},
		},
	}

	completeBytes, err := xml.Marshal(completeUploads)
	c.Assert(err, IsNil)
	// Indicating that all parts are uploaded and initiating completeMultipartUpload.
	request, err = newTestRequest("POST", getCompleteMultipartUploadURL(s.endPoint, bucketName, objectName, uploadID),
		int64(len(completeBytes)), bytes.NewReader(completeBytes), s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// Execute the complete multipart request.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// verify whether complete multipart was successfull.
	c.Assert(response.StatusCode, Equals, http.StatusOK)
}

// TestObjectMultipartOverwriteSinglePut - Initiates a NewMultipart upload, uploads 2 parts,
// completes the multipart upload and validates the status of the operation.
// then, after PutObject with same object name on the bucket,
// test validates for successful overwrite.
func (s *TestSuiteXL) TestObjectMultipartOverwriteSinglePut(c *C) {
	// generate a random bucket name.
	bucketName := getRandomBucketName()
	// HTTP request to create the bucket.
	request, err := newTestRequest("PUT", getMakeBucketURL(s.endPoint, bucketName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client := http.Client{}
	// execute the HTTP request to create bucket.
	response, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, 200)

	objectName := "test-multipart-object"

	request, err = newTestRequest("POST", getNewMultipartURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request initiating the new multipart upload.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// expecting the response status code to be http.StatusOK(200 OK).
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// parse the response body and obtain the new upload ID.
	decoder := xml.NewDecoder(response.Body)
	newResponse := &InitiateMultipartUploadResponse{}

	err = decoder.Decode(newResponse)
	c.Assert(err, IsNil)
	c.Assert(len(newResponse.UploadID) > 0, Equals, true)
	// uploadID to be used for rest of the multipart operations on the object.
	uploadID := newResponse.UploadID

	// content for the part to be uploaded.
	// Create a byte array of 5MB.
	data := bytes.Repeat([]byte("0123456789abcdef"), 5*1024*1024/16)
	// calculate md5Sum of the data.
	hasher := md5.New()
	hasher.Write(data)
	md5Sum := hasher.Sum(nil)

	buffer1 := bytes.NewReader(data)
	// HTTP request for the part to be uploaded.
	request, err = newTestRequest("PUT", getPartUploadURL(s.endPoint, bucketName, objectName, uploadID, "1"),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	// set the Content-Md5 header to the base64 encoding the md5Sum of the content.
	request.Header.Set("Content-Md5", base64.StdEncoding.EncodeToString(md5Sum))
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to upload the first part.
	response1, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response1.StatusCode, Equals, http.StatusOK)

	// Create a byte array of 1 byte.
	// content for the second part to be uploaded.
	data = []byte("0")

	hasher = md5.New()
	hasher.Write(data)
	// calculate md5Sum of the data.
	md5Sum = hasher.Sum(nil)

	buffer2 := bytes.NewReader(data)
	// HTTP request for the second part to be uploaded.
	request, err = newTestRequest("PUT", getPartUploadURL(s.endPoint, bucketName, objectName, uploadID, "2"),
		int64(buffer2.Len()), buffer2, s.accessKey, s.secretKey)
	// set the Content-Md5 header to the base64 encoding the md5Sum of the content.
	request.Header.Set("Content-Md5", base64.StdEncoding.EncodeToString(md5Sum))
	c.Assert(err, IsNil)

	client = http.Client{}
	// execute the HTTP request to upload the second part.
	response2, err := client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response2.StatusCode, Equals, http.StatusOK)

	// Complete multipart upload.
	completeUploads := &completeMultipartUpload{
		Parts: []completePart{
			{
				PartNumber: 1,
				ETag:       response1.Header.Get("ETag"),
			},
			{
				PartNumber: 2,
				ETag:       response2.Header.Get("ETag"),
			},
		},
	}

	completeBytes, err := xml.Marshal(completeUploads)
	c.Assert(err, IsNil)
	// Indicating that all parts are uploaded and initiating completeMultipartUpload.
	request, err = newTestRequest("POST", getCompleteMultipartUploadURL(s.endPoint, bucketName, objectName, uploadID),
		int64(len(completeBytes)), bytes.NewReader(completeBytes), s.accessKey, s.secretKey)
	c.Assert(err, IsNil)

	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	// A new object is uploaded with same key as before.
	// validation for successful overwrite is done by downloading back the object and reading its content.
	buffer1 = bytes.NewReader([]byte("hello world"))
	// HTTP request for uploading the object.
	request, err = newTestRequest("PUT", getPutObjectURL(s.endPoint, bucketName, objectName),
		int64(buffer1.Len()), buffer1, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request to upload the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	// verify whether upload was successfull.
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// HTTP request for downloading the object.
	request, err = newTestRequest("GET", getGetObjectURL(s.endPoint, bucketName, objectName),
		0, nil, s.accessKey, s.secretKey)
	c.Assert(err, IsNil)
	// execute the HTTP request to download the object.
	response, err = client.Do(request)
	c.Assert(err, IsNil)
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	// validate the response content length.
	c.Assert(response.ContentLength, Equals, int64(len([]byte("hello world"))))
	var buffer3 bytes.Buffer
	// read the response body to obtain the content of downloaded object.
	n, err := io.Copy(&buffer3, response.Body)
	c.Assert(err, IsNil)
	// validate the downloaded content.
	// verify successful overwrite with the new content.
	c.Assert(n, Equals, int64(len([]byte("hello world"))))
	c.Assert(true, Equals, bytes.Equal(buffer3.Bytes(), []byte("hello world")))
}
