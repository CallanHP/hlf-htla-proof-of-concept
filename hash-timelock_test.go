package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

func TestCrossChannelConfirmation(t *testing.T) {
	channelOne := shim.NewMockStub("channelOne", new(HashTimeLockContract))
	if channelOne == nil {
		t.Fatalf("channelOne creation failed")
	}
	channelTwo := shim.NewMockStub("channelTwo", new(HashTimeLockContract))
	if channelTwo == nil {
		t.Fatalf("channelTwo creation failed")
	}

	//Our hash, alg and pre-image - in this scenario, Alice (A) has been given the
	//hash and hashAlg by Charlie(C), Charlie alone knows the pre-image
	preImage := "test_hash"
	hash := "5a32f0967623012cdd4c29257f808f3f209184e992c39dc6d931f89831e7b1eb9379f9e3a20da09eb06d0ca53bd9c0845dda91baed17a713c0cac8a24259c0b9"
	hashAlg := "SHA512"

	//Create our proposal in channel one.
	testProposal := "{" +
		"\"proposalId\": \"prop1234\"," +
		"\"proposalHandler\": \"Bob\"" +
		"}"
	args := [][]byte{[]byte("createProposal"), []byte(testProposal), []byte(hash), []byte(hashAlg)}
	res := channelOne.MockInvoke("txid1", args)
	if res.Status != 200 {
		t.Errorf("Create Proposal channel one returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}
	//Have the channel one 'handler' catch the proposal event
	channelOneProposalEvent := <-channelOne.ChaincodeEventsChannel
	if channelOneProposalEvent == nil {
		t.Error("No proposal handler event fired!")
	}
	if channelOneProposalEvent.EventName != "Bob"+ProposalCreatedHandlerEvent {
		t.Errorf("Create proposal fired event with name %s, but expected %s.", channelOneProposalEvent.EventName, "Bob"+ProposalCreatedHandlerEvent)
	}
	channelOneProposalCreatedEvent := ProposalCreatedEventObject{}
	err := json.Unmarshal(channelOneProposalEvent.Payload, &channelOneProposalCreatedEvent)
	if err != nil {
		t.Error("Error while unmarshalling event fired when creating proposal in channel one.")
	}
	//Clear the events channel
	_ = <-channelOne.ChaincodeEventsChannel

	//Handler then creates the proposal in channel two, using the values from the event
	//In reality, would probably retrieve the proposal and hash from channel one, but there are no
	//functions for that in this PoC...
	testProposal = "{" +
		"\"proposalId\": \"" + channelOneProposalCreatedEvent.ProposalID + "\"," +
		"\"proposalHandler\": \"Charlie\"" +
		"}"
	args = [][]byte{[]byte("createProposal"), []byte(testProposal), []byte(hash), []byte(hashAlg)}
	res = channelTwo.MockInvoke("txid1", args)
	if res.Status != 200 {
		t.Errorf("Create Proposal channel two returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}

	//Channel two 'handler' catches this proposal - then supplies the pre-image to confirm
	channelTwoProposalEvent := <-channelTwo.ChaincodeEventsChannel
	if channelTwoProposalEvent == nil {
		t.Error("No proposal handler event fired!")
	}
	if channelTwoProposalEvent.EventName != "Charlie"+ProposalCreatedHandlerEvent {
		t.Errorf("Create proposal fired event with name %s, but expected %s.", channelTwoProposalEvent.EventName, "Charlie"+ProposalCreatedHandlerEvent)
	}
	channelTwoProposalCreatedEvent := ProposalCreatedEventObject{}
	err = json.Unmarshal(channelTwoProposalEvent.Payload, &channelTwoProposalCreatedEvent)
	if err != nil {
		t.Error("Error while unmarshalling event fired when creating proposal in channel two.")
	}
	//Clear the events channel
	_ = <-channelTwo.ChaincodeEventsChannel

	//Since we recieved a proposal that we know about (presumably, there needs
	//to be some logic to establish this), we can supply the pre-image to confirm
	args = [][]byte{[]byte("confirmProposal"), []byte("prop1234"), []byte(preImage)}
	res = channelTwo.MockInvoke("txid2", args)
	if res.Status != 200 {
		t.Errorf("Confirm Proposal returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}
	//Channel one 'handler' catches the confirmation event, then replays it into channel one
	channelTwoProposalConfirmed := <-channelTwo.ChaincodeEventsChannel
	if channelOneProposalEvent == nil {
		t.Error("No proposal confirmation event fired!")
	}
	if channelTwoProposalConfirmed.EventName != ProposalConfirmedHandlerEvent {
		t.Errorf("Confirm proposal fired event with name %s, but expected %s.", channelTwoProposalConfirmed.EventName, ProposalConfirmedHandlerEvent)
	}
	channelTwoProposalConfirmEvent := ProposalConfirmedEventObject{}
	err = json.Unmarshal(channelTwoProposalConfirmed.Payload, &channelTwoProposalConfirmEvent)
	if err != nil {
		t.Error("Error while unmarshalling event fired when confirming proposal in channel two.")
	}
	//Use the hash in that event to confirm the proposal in channel one
	args = [][]byte{[]byte("confirmProposal"), []byte("prop1234"), []byte(channelTwoProposalConfirmEvent.PreImage)}
	res = channelOne.MockInvoke("txid2", args)
	if res.Status != 200 {
		t.Errorf("Confirm Proposal returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}
	//Proposal should be confirmed in channel one, using the pre-image supplied by B, based upon
	//the confirmation provided by C in channel two.
	expectedRes := "{\"proposal\":{\"proposalId\":\"prop1234\",\"proposalHandler\":\"Bob\"},\"status\":\"CONFIRMED\"," +
		"\"hash\":\"5a32f0967623012cdd4c29257f808f3f209184e992c39dc6d931f89831e7b1eb9379f9e3a20da09eb06d0ca53bd9c0845dda91baed17a713c0cac8a24259c0b9\"," +
		"\"hashAlgorithm\":\"SHA512\"}"
	proposal, err := channelOne.GetState(proposalPrefix + "prop1234")
	if err != nil {
		t.Error("Error getting proposal by id from mock stub")
	}
	if string(proposal) != expectedRes {
		t.Errorf("Channel one proposal final result: %s, but expected: %s.", string(proposal), expectedRes)
	}
}

func TestCreateProposalSuccess(t *testing.T) {
	stub := shim.NewMockStub("mockChaincodeStub", new(HashTimeLockContract))
	if stub == nil {
		t.Fatalf("MockStub creation failed")
	}
	testProposal := "{" +
		"\"proposalId\": \"prop1234\"," +
		"\"proposalHandler\": \"Bob\"" +
		"}"
	args := [][]byte{[]byte("createProposal"), []byte(testProposal), []byte("hash"), []byte("SHA512")}
	res := stub.MockInvoke("txid1", args)
	if res.Status != 200 {
		t.Errorf("Create Proposal returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}
	//Check that the object was created
	expectedRes := "{\"proposal\":{\"proposalId\":\"prop1234\",\"proposalHandler\":\"Bob\"},\"status\":\"PENDING\",\"hash\":\"hash\",\"hashAlgorithm\":\"SHA512\"}"
	proposal, err := stub.GetState(proposalPrefix + "prop1234")
	if err != nil {
		t.Error("Error getting proposal by id from mock stub")
	}
	if string(proposal) != expectedRes {
		t.Errorf("Create proposal created %s, but expected: %s.", string(proposal), expectedRes)
	}
	//Check if events were fired
	expectedEvent := "{\"proposalId\":\"prop1234\"}"
	proposalHandlerEvent := <-stub.ChaincodeEventsChannel
	if proposalHandlerEvent == nil {
		t.Error("No proposal handler event fired!")
	}
	if proposalHandlerEvent.EventName != "Bob"+ProposalCreatedHandlerEvent {
		t.Errorf("Create proposal fired event with name %s, but expected %s.", proposalHandlerEvent.EventName, "Bob"+ProposalCreatedHandlerEvent)
	}
	if string(proposalHandlerEvent.Payload) != expectedEvent {
		t.Errorf("Create proposal fired event with payload %s, but expected %s.", string(proposalHandlerEvent.Payload), expectedEvent)
	}
	proposalTimeoutEvent := <-stub.ChaincodeEventsChannel
	if proposalTimeoutEvent == nil {
		t.Error("No proposal timeout event fired!")
	}
	if proposalTimeoutEvent.EventName != ProposalCreateTimeoutEvent {
		t.Errorf("Create proposal fired timeout event with name %s, but expected %s.", proposalTimeoutEvent.EventName, ProposalCreateTimeoutEvent)
	}
	if string(proposalTimeoutEvent.Payload) != expectedEvent {
		t.Errorf("Create proposal fired timeout event with payload %s, but expected %s.", string(proposalTimeoutEvent.Payload), expectedEvent)
	}
}

func TestCreateProposalInvalidArgs(t *testing.T) {
	stub := shim.NewMockStub("mockChaincodeStub", new(HashTimeLockContract))
	if stub == nil {
		t.Fatalf("MockStub creation failed")
	}
	testProposal := "{" +
		"\"proposalId\": \"prop1234\"," +
		"\"proposalHandler\": \"Bob\"" +
		"}"
	args := [][]byte{[]byte("createProposal"), []byte(testProposal), []byte("hash")}
	res := stub.MockInvoke("txid1", args)
	if res.Status != 500 {
		t.Errorf("Create Proposal returned OK status, got: %d, want: %d.", res.Status, 500)
	}
	//Check that the error message is appropriate
	expectedMessage := "Invalid arguments to createProposal, expected proposal, hash, hashingAlg."
	if res.Message != expectedMessage {
		t.Errorf("Expected Error: %s, got: %s", expectedMessage, res.Message)
	}
}

func TestCreateProposalInvalidPropId(t *testing.T) {
	stub := shim.NewMockStub("mockChaincodeStub", new(HashTimeLockContract))
	if stub == nil {
		t.Fatalf("MockStub creation failed")
	}
	testProposal := "{" +
		"\"proposalHandler\": \"Bob\"" +
		"}"
	args := [][]byte{[]byte("createProposal"), []byte(testProposal), []byte("hash"), []byte("SHA512")}
	res := stub.MockInvoke("txid1", args)
	if res.Status != 500 {
		t.Errorf("Create Proposal returned OK status, got: %d, want: %d.", res.Status, 500)
	}
	//Check that the error message is appropriate
	expectedMessage := "No proposalId provided as part of proposal."
	if res.Message != expectedMessage {
		t.Errorf("Expected Error: %s, got: %s", expectedMessage, res.Message)
	}
}

func TestCreateProposalInvalidHandler(t *testing.T) {
	stub := shim.NewMockStub("mockChaincodeStub", new(HashTimeLockContract))
	if stub == nil {
		t.Fatalf("MockStub creation failed")
	}
	testProposal := "{" +
		"\"proposalId\": \"prop1234\"" +
		"}"
	args := [][]byte{[]byte("createProposal"), []byte(testProposal), []byte("hash"), []byte("SHA512")}
	res := stub.MockInvoke("txid1", args)
	if res.Status != 500 {
		t.Errorf("Create Proposal returned OK status, got: %d, want: %d.", res.Status, 500)
	}
	//Check that the error message is appropriate
	expectedMessage := "No proposalHandler provided as part of proposal."
	if res.Message != expectedMessage {
		t.Errorf("Expected Error: %s, got: %s", expectedMessage, res.Message)
	}
}

func TestCreateProposalInvalidHashAlg(t *testing.T) {
	stub := shim.NewMockStub("mockChaincodeStub", new(HashTimeLockContract))
	if stub == nil {
		t.Fatalf("MockStub creation failed")
	}
	testProposal := "{" +
		"\"proposalId\": \"prop1234\"" +
		"}"
	args := [][]byte{[]byte("createProposal"), []byte(testProposal), []byte("hash"), []byte("My-Awesome-Hashing-Alg")}
	res := stub.MockInvoke("txid1", args)
	if res.Status != 500 {
		t.Errorf("Create Proposal returned OK status, got: %d, want: %d.", res.Status, 500)
	}
	//Check that the error message is appropriate
	expectedMessage := "Only these hashing algorithms are supported: " + strings.Join(validHashingAlgorithms, ", ")
	if res.Message != expectedMessage {
		t.Errorf("Expected Error: %s, got: %s", expectedMessage, res.Message)
	}
}

func TestConfirmProposalSuccess(t *testing.T) {
	stub := shim.NewMockStub("mockChaincodeStub", new(HashTimeLockContract))
	if stub == nil {
		t.Fatalf("MockStub creation failed")
	}
	//Create a proposal as a pre-req
	testProposal := "{" +
		"\"proposalId\": \"prop1234\"," +
		"\"proposalHandler\": \"Bob\"" +
		"}"
	preImage := "test_hash"
	hash := "5a32f0967623012cdd4c29257f808f3f209184e992c39dc6d931f89831e7b1eb9379f9e3a20da09eb06d0ca53bd9c0845dda91baed17a713c0cac8a24259c0b9"
	args := [][]byte{[]byte("createProposal"), []byte(testProposal), []byte(hash), []byte("SHA512")}
	res := stub.MockInvoke("txid1", args)
	if res.Status != 200 {
		t.Errorf("Create Proposal returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}
	//Clear the event channel
	_, _ = <-stub.ChaincodeEventsChannel, <-stub.ChaincodeEventsChannel

	//Run the confirmation
	args = [][]byte{[]byte("confirmProposal"), []byte("prop1234"), []byte(preImage)}
	res = stub.MockInvoke("txid2", args)
	if res.Status != 200 {
		t.Errorf("Confirm Proposal returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}
	//Check that the object was updated
	expectedRes := "{\"proposal\":{\"proposalId\":\"prop1234\",\"proposalHandler\":\"Bob\"},\"status\":\"CONFIRMED\"," +
		"\"hash\":\"5a32f0967623012cdd4c29257f808f3f209184e992c39dc6d931f89831e7b1eb9379f9e3a20da09eb06d0ca53bd9c0845dda91baed17a713c0cac8a24259c0b9\"," +
		"\"hashAlgorithm\":\"SHA512\"}"
	proposal, err := stub.GetState(proposalPrefix + "prop1234")
	if err != nil {
		t.Error("Error getting proposal by id from mock stub")
	}
	if string(proposal) != expectedRes {
		t.Errorf("Confirm proposal created %s, but expected: %s.", string(proposal), expectedRes)
	}
	//Check if events were fired
	expectedEvent := "{\"proposalId\":\"prop1234\",\"preImage\":\"test_hash\"}"
	proposalConfirmationEvent := <-stub.ChaincodeEventsChannel
	if proposalConfirmationEvent == nil {
		t.Error("No proposal handler event fired!")
	}
	if proposalConfirmationEvent.EventName != ProposalConfirmedHandlerEvent {
		t.Errorf("Create proposal fired event with name %s, but expected %s.", proposalConfirmationEvent.EventName, ProposalConfirmedHandlerEvent)
	}
	if string(proposalConfirmationEvent.Payload) != expectedEvent {
		t.Errorf("Create proposal fired event with payload %s, but expected %s.", string(proposalConfirmationEvent.Payload), expectedEvent)
	}
}

func TestConfirmProposalInvalidArgs(t *testing.T) {
	stub := shim.NewMockStub("mockChaincodeStub", new(HashTimeLockContract))
	if stub == nil {
		t.Fatalf("MockStub creation failed")
	}
	args := [][]byte{[]byte("confirmProposal"), []byte("test_id")}
	res := stub.MockInvoke("txid1", args)
	if res.Status != 500 {
		t.Errorf("Confirm Proposal returned OK status, got: %d, want: %d.", res.Status, 500)
	}
	//Check that the error message is appropriate
	expectedMessage := "Invalid arguments to confirmProposal, expected proposalId, pre-image."
	if res.Message != expectedMessage {
		t.Errorf("Expected Error: %s, got: %s", expectedMessage, res.Message)
	}
}

func TestConfirmProposalNoSuchProposal(t *testing.T) {
	stub := shim.NewMockStub("mockChaincodeStub", new(HashTimeLockContract))
	if stub == nil {
		t.Fatalf("MockStub creation failed")
	}
	preImage := "test_hash"
	//Run the confirmation
	args := [][]byte{[]byte("confirmProposal"), []byte("prop1234"), []byte(preImage)}
	res := stub.MockInvoke("txid2", args)
	if res.Status != 500 {
		t.Errorf("Confirm Proposal returned OK status, got: %d, want: %d.", res.Status, 500)
	}
	//Check that the error message is appropriate
	expectedMessage := "No such proposal. It may have expired and been invalidated."
	if res.Message != expectedMessage {
		t.Errorf("Expected Error: %s, got: %s", expectedMessage, res.Message)
	}
}

func TestConfirmProposalWithSHA256(t *testing.T) {
	stub := shim.NewMockStub("mockChaincodeStub", new(HashTimeLockContract))
	if stub == nil {
		t.Fatalf("MockStub creation failed")
	}
	//Create a proposal as a pre-req
	testProposal := "{" +
		"\"proposalId\": \"prop1234\"," +
		"\"proposalHandler\": \"Bob\"" +
		"}"
	preImage := "test_hash"
	hash := "6b70a820eb978882fa49b199c853a5676e5e1a4744371be5affd4b3af1f5dde6"
	args := [][]byte{[]byte("createProposal"), []byte(testProposal), []byte(hash), []byte("SHA256")}
	res := stub.MockInvoke("txid1", args)
	if res.Status != 200 {
		t.Errorf("Create Proposal returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}

	//Run the confirmation
	args = [][]byte{[]byte("confirmProposal"), []byte("prop1234"), []byte(preImage)}
	res = stub.MockInvoke("txid2", args)
	if res.Status != 200 {
		t.Errorf("Confirm Proposal returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}
}

func TestConfirmProposalInvalidHash(t *testing.T) {
	stub := shim.NewMockStub("mockChaincodeStub", new(HashTimeLockContract))
	if stub == nil {
		t.Fatalf("MockStub creation failed")
	}
	//Create a proposal as a pre-req
	testProposal := "{" +
		"\"proposalId\": \"prop1234\"," +
		"\"proposalHandler\": \"Bob\"" +
		"}"
	preImage := "test_hash"
	hash := "not_hash"
	args := [][]byte{[]byte("createProposal"), []byte(testProposal), []byte(hash), []byte("SHA512")}
	res := stub.MockInvoke("txid1", args)
	if res.Status != 200 {
		t.Errorf("Create Proposal returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}

	//Run the confirmation
	args = [][]byte{[]byte("confirmProposal"), []byte("prop1234"), []byte(preImage)}
	res = stub.MockInvoke("txid2", args)
	if res.Status != 500 {
		t.Errorf("Confirm Proposal returned OK status, got: %d, want: %d.", res.Status, 500)

	}
	//Check that the error message is appropriate
	expectedMessage := "Invalid Pre-image supplied."
	if res.Message != expectedMessage {
		t.Errorf("Expected Error: %s, got: %s", expectedMessage, res.Message)
	}
	//Check that the proposal wasn't updated
	proposalBytes, err := stub.GetState(proposalPrefix + "prop1234")
	if err != nil {
		t.Error("Error getting proposal by id from mock stub")
	}
	proposal := proposalEntry{}
	err = json.Unmarshal(proposalBytes, &proposal)
	if err != nil {
		t.Error("Error parsing proposal bytes into the proposal object")
	}
	if proposal.Status != PendingStatus {
		t.Errorf("Proposal confirmed with invalid hash should sill be in status %s, instead in %s", PendingStatus, proposal.Status)
	}
}

func TestConfirmProposalUsingUppercaseHashString(t *testing.T) {
	stub := shim.NewMockStub("mockChaincodeStub", new(HashTimeLockContract))
	if stub == nil {
		t.Fatalf("MockStub creation failed")
	}
	//Create a proposal as a pre-req
	testProposal := "{" +
		"\"proposalId\": \"prop1234\"," +
		"\"proposalHandler\": \"Bob\"" +
		"}"
	preImage := "test_hash"
	hash := "6B70A820EB978882FA49B199C853A5676E5E1A4744371BE5AFFD4B3AF1F5DDE6"
	args := [][]byte{[]byte("createProposal"), []byte(testProposal), []byte(hash), []byte("SHA256")}
	res := stub.MockInvoke("txid1", args)
	if res.Status != 200 {
		t.Errorf("Create Proposal returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}

	//Run the confirmation
	args = [][]byte{[]byte("confirmProposal"), []byte("prop1234"), []byte(preImage)}
	res = stub.MockInvoke("txid2", args)
	if res.Status != 200 {
		t.Errorf("Confirm Proposal returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}
}

func TestConfirmProposalWithSHA384(t *testing.T) {
	stub := shim.NewMockStub("mockChaincodeStub", new(HashTimeLockContract))
	if stub == nil {
		t.Fatalf("MockStub creation failed")
	}
	//Create a proposal as a pre-req
	testProposal := "{" +
		"\"proposalId\": \"prop1234\"," +
		"\"proposalHandler\": \"Bob\"" +
		"}"
	preImage := "test_hash"
	hash := "708af8efbb882bb662a5a5f19d3164133621266903cec7ee0ce9eca950a7b7f8d09defedb4474da4257274741f2a07a8"
	args := [][]byte{[]byte("createProposal"), []byte(testProposal), []byte(hash), []byte("SHA384")}
	res := stub.MockInvoke("txid1", args)
	if res.Status != 200 {
		t.Errorf("Create Proposal returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}

	//Run the confirmation
	args = [][]byte{[]byte("confirmProposal"), []byte("prop1234"), []byte(preImage)}
	res = stub.MockInvoke("txid2", args)
	if res.Status != 200 {
		t.Errorf("Confirm Proposal returned non-OK status, got: %d, want: %d.", res.Status, 200)
		t.Errorf("Error - %s", res.Message)
	}
}
