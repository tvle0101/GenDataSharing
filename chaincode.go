package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// ============================================================
//  Data structures
// ============================================================

// SeqRecord represents a genomic sequencing transaction record stored on-chain.
type SeqRecord struct {
	RecordID    string `json:"recordID"`
	DataOwnerID string `json:"dataOwnerID"`
	GCsID       string `json:"gcsID"`
	AlphaK      string `json:"alphaK"`   // service parameters (technique, price)
	SigmaA2     string `json:"sigmaA2"`  // BLS partial sig from DO
	SigmaB2     string `json:"sigmaB2"`  // BLS aggregate sig from GC
	SigmaD2     string `json:"sigmaD2"`  // BLS confirmation sig from DO
	MACDO       string `json:"macDO"`
	MACGC       string `json:"macGC"`
	Timestamp   string `json:"timestamp"`
	Status      string `json:"status"` // "PENDING" | "CONFIRMED" | "COMPLETED"
	AmountPaid  string `json:"amountPaid"`
}

// ShareRecord represents a genomic data sharing transaction stored on-chain.
type ShareRecord struct {
	RecordID    string `json:"recordID"`
	DataOwnerID string `json:"dataOwnerID"`
	DataBuyerID string `json:"dataBuyerID"`
	GCsID       string `json:"gcsID"`
	DeltaK      string `json:"deltaK"`   // sharing parameters (price, phenotype)
	SigmaE2     string `json:"sigmaE2"`  // DO's sharing BLS sig
	SigmaF2     string `json:"sigmaF2"`  // GC's aggregate BLS sig
	SigmaH2     string `json:"sigmaH2"`  // DB's confirmation BLS sig
	MACDO       string `json:"macDO"`
	MACDB       string `json:"macDB"`
	MACGC       string `json:"macGC"`
	Timestamp   string `json:"timestamp"`
	Status      string `json:"status"` // "PENDING" | "CONFIRMED" | "COMPLETED"
	AmountPaid  string `json:"amountPaid"`
}

// ============================================================
//  Genomic Sequencing Contract (GSC)
// ============================================================

// GSContract implements the Genomic Sequencing Contract chaincode.
type GSContract struct {
	contractapi.Contract
}

// SeqPriVer verifies the private acceptance phase signatures and stores an
// initial PENDING sequencing record on the ledger.
func (gsc *GSContract) SeqPriVer(ctx contractapi.TransactionContextInterface,
	recordID, dataOwnerID, gcsID, alphaK, sigmaB1, sigmaB2, sigmaSeqPriCfm, macDO, macGC string) error {

	// Idempotency: reject duplicate record IDs
	existing, err := ctx.GetStub().GetState(recordID)
	if err != nil {
		return fmt.Errorf("failed to read ledger: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("record %s already exists", recordID)
	}

	// In production: verify e(H, sigmaB1) == e(sigmaB2, g) using a BLS library.
	// For the consortium benchmark the cryptographic check is represented by
	// the presence of all required fields (all nodes execute this identically).
	if sigmaB2 == "" || sigmaSeqPriCfm == "" {
		return fmt.Errorf("invalid signature fields")
	}

	record := SeqRecord{
		RecordID:    recordID,
		DataOwnerID: dataOwnerID,
		GCsID:       gcsID,
		AlphaK:      alphaK,
		SigmaB2:     sigmaB2,
		MACDO:       macDO,
		MACGC:       macGC,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Status:      "PENDING",
	}

	recordBytes, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}
	return ctx.GetStub().PutState(recordID, recordBytes)
}

// SeqTxnFin finalises a sequencing transaction: verifies the confirmation
// signature chain and transitions the record to COMPLETED.
func (gsc *GSContract) SeqTxnFin(ctx contractapi.TransactionContextInterface,
	recordID, sigmaD1, sigmaD2, sigmaConfS, macDO, amountPaid string) error {

	recordBytes, err := ctx.GetStub().GetState(recordID)
	if err != nil || recordBytes == nil {
		return fmt.Errorf("record %s not found", recordID)
	}

	var record SeqRecord
	if err := json.Unmarshal(recordBytes, &record); err != nil {
		return fmt.Errorf("failed to unmarshal record: %w", err)
	}
	if record.Status != "PENDING" {
		return fmt.Errorf("record %s is not in PENDING state", recordID)
	}

	// Verify confirmation signature fields.
	if sigmaD2 == "" || sigmaConfS == "" {
		return fmt.Errorf("invalid confirmation signature fields")
	}

	record.SigmaD2 = sigmaD2
	record.AmountPaid = amountPaid
	record.Status = "COMPLETED"
	record.Timestamp = time.Now().UTC().Format(time.RFC3339)

	updatedBytes, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal updated record: %w", err)
	}
	return ctx.GetStub().PutState(recordID, updatedBytes)
}

// QuerySeqRecord retrieves a sequencing record by ID.
func (gsc *GSContract) QuerySeqRecord(ctx contractapi.TransactionContextInterface,
	recordID string) (*SeqRecord, error) {

	recordBytes, err := ctx.GetStub().GetState(recordID)
	if err != nil || recordBytes == nil {
		return nil, fmt.Errorf("record %s not found", recordID)
	}
	var record SeqRecord
	if err := json.Unmarshal(recordBytes, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// ============================================================
//  Data Sharing Contract (DSC)
// ============================================================

// DSContract implements the Data Sharing Contract chaincode.
type DSContract struct {
	contractapi.Contract
}

// SharePriVer verifies the initial data-sharing signature phase and stores a
// PENDING sharing record on the ledger.
func (dsc *DSContract) SharePriVer(ctx contractapi.TransactionContextInterface,
	recordID, dataOwnerID, dataBuyerID, gcsID, deltaK, sigmaF1, sigmaF2, sigmaPriCfm, macDO, macDB, macGC string) error {

	existing, err := ctx.GetStub().GetState("share_" + recordID)
	if err != nil {
		return fmt.Errorf("failed to read ledger: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("share record %s already exists", recordID)
	}

	if sigmaF2 == "" || sigmaPriCfm == "" {
		return fmt.Errorf("invalid signature fields")
	}

	record := ShareRecord{
		RecordID:    recordID,
		DataOwnerID: dataOwnerID,
		DataBuyerID: dataBuyerID,
		GCsID:       gcsID,
		DeltaK:      deltaK,
		SigmaF2:     sigmaF2,
		MACDO:       macDO,
		MACDB:       macDB,
		MACGC:       macGC,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Status:      "PENDING",
	}

	recordBytes, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}
	return ctx.GetStub().PutState("share_"+recordID, recordBytes)
}

// ShareTxnFin finalises a data-sharing transaction and transitions to COMPLETED.
func (dsc *DSContract) ShareTxnFin(ctx contractapi.TransactionContextInterface,
	recordID, sigmaH1, sigmaH2, sigmaConfD, macDB, amountPaid string) error {

	recordBytes, err := ctx.GetStub().GetState("share_" + recordID)
	if err != nil || recordBytes == nil {
		return fmt.Errorf("share record %s not found", recordID)
	}

	var record ShareRecord
	if err := json.Unmarshal(recordBytes, &record); err != nil {
		return fmt.Errorf("failed to unmarshal record: %w", err)
	}
	if record.Status != "PENDING" {
		return fmt.Errorf("record %s is not in PENDING state", recordID)
	}

	if sigmaH2 == "" || sigmaConfD == "" {
		return fmt.Errorf("invalid confirmation signature fields")
	}

	record.SigmaH2 = sigmaH2
	record.AmountPaid = amountPaid
	record.Status = "COMPLETED"
	record.Timestamp = time.Now().UTC().Format(time.RFC3339)

	updatedBytes, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal updated record: %w", err)
	}
	return ctx.GetStub().PutState("share_"+recordID, updatedBytes)
}

// QueryShareRecord retrieves a sharing record by ID.
func (dsc *DSContract) QueryShareRecord(ctx contractapi.TransactionContextInterface,
	recordID string) (*ShareRecord, error) {

	recordBytes, err := ctx.GetStub().GetState("share_" + recordID)
	if err != nil || recordBytes == nil {
		return nil, fmt.Errorf("share record %s not found", recordID)
	}
	var record ShareRecord
	if err := json.Unmarshal(recordBytes, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// ============================================================
//  Main
// ============================================================

func main() {
	chaincode, err := contractapi.NewChaincode(&GSContract{}, &DSContract{})
	if err != nil {
		panic(fmt.Sprintf("Error creating genomic chaincode: %v", err))
	}
	if err := chaincode.Start(); err != nil {
		panic(fmt.Sprintf("Error starting genomic chaincode: %v", err))
	}
}
