package main

import (
	"os"
	"log"
	"fmt"
	"errors"
	"encoding/binary"

	bolt "go.etcd.io/bbolt"	
	"google.golang.org/protobuf/proto"
	pb "example.com/meshproxy-go/gomeshproto"
)

var db *bolt.DB

func InitDb() {
	var err error
	db, err = bolt.Open("database.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}

	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("nodes"))
		if err != nil {
			fmt.Printf("create bucket error: %s", err)
			os.Exit(1)
		}
		_, err = tx.CreateBucketIfNotExists([]byte("nodeindex"))
		if err != nil {
			fmt.Printf("create bucket error: %s", err)
			os.Exit(1)
		}
		return nil
	})
}

func CloseDb() {
	db.Close()
}

func DumpDb(dumpFlag string) {
	if dumpFlag == "nodes" {
		DumpNodes()
	} else if dumpFlag == "nodeindex" {
		DumpNodeIndex()
	}
}

func DumpNodes() {
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("nodes"))
		c := b.Cursor()
		counter := 0
		for k, v := c.First(); k != nil; k, v = c.Next() {
			counter++
			nodeInfo := pb.NodeInfo{}
			if err := proto.Unmarshal(v, &nodeInfo); err != nil {
				fmt.Printf("Error unmarshalling: %+v\n", err)
			} else {
				fmt.Printf("%s -> %s\n\n", k, nodeInfo.String())
			}
		}
		fmt.Printf("Node count: %d\n", counter)
		return nil
	})
}

func DumpNodeIndex() {
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("nodeindex"))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			num := binary.LittleEndian.Uint32(k)
			fmt.Printf("%+v %+v\n", num, string(v))
		}
		return nil
	})
}

func IndexDb() {
	db.Update(func(tx *bolt.Tx) error {
		n := tx.Bucket([]byte("nodes"))
		nni := tx.Bucket([]byte("nodeindex"))
		c := n.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			nodeInfo := pb.NodeInfo{}
			if err := proto.Unmarshal(v, &nodeInfo); err != nil {
				fmt.Printf("Error unmarshalling: %+v\n", err)
				log.Fatal("not good")
			}
			bs := make([]byte, 4)
    	binary.LittleEndian.PutUint32(bs, nodeInfo.Num)
			nni.Put(bs, k)
		}
		return nil
	})
}

func IterateNodesFromDb(cb func(n []byte)) {
	db.View(func(tx *bolt.Tx) error {

		b := tx.Bucket([]byte("nodes"))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			nodeInfo := pb.NodeInfo{}
			if err := proto.Unmarshal(v, &nodeInfo); err != nil {
				log.Fatal(err)
			}

			if nodeInfo.User.Id == connectedNodeId {
				//fmt.Printf("skipping own node\n")
				continue
			}

			nodeInfoPayload := pb.FromRadio{PayloadVariant: &pb.FromRadio_NodeInfo{&nodeInfo}}
			out, err := proto.Marshal(&nodeInfoPayload)
			if err != nil {
				log.Fatal(err)
			}
			cb(out)
		}

		return nil
	})
}

func FetchNodeInfoByNumber(nodeNumber uint32) (pb.NodeInfo, error) {

	//fmt.Printf("fetchNodebynum %d\n", nodeNumber)
	nodeInfo := pb.NodeInfo{}
	err := db.View(func(tx *bolt.Tx) error {
		bi := tx.Bucket([]byte("nodeindex"))
		k := make([]byte, 4)
    binary.LittleEndian.PutUint32(k, nodeNumber)
		data := bi.Get(k)
		nodeId := string(data)

		b := tx.Bucket([]byte("nodes"))
		data = b.Get([]byte(nodeId))
		if data == nil || len(data) == 0 {
			return errors.New("node not found")
		}
		if err := proto.Unmarshal(data, &nodeInfo); err != nil {
			return err
		}
		return nil
	})
	return nodeInfo, err
}