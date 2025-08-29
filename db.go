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
    _, err = tx.CreateBucketIfNotExists([]byte("messages"))
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
  } else if dumpFlag == "messages" {
    DumpMessages()
  }
}

func DumpMessages() {
  db.View(func(tx *bolt.Tx) error {
    b := tx.Bucket([]byte("messages"))
    c := b.Cursor()
    for k, v := c.First(); k != nil; k, v = c.Next() {
			//fmt.Printf("%+v %+v\n", k, v)
      pkt := pb.FromRadio{}
      if err := proto.Unmarshal(v[4:], &pkt); err != nil {
        fmt.Printf("Error unmarshalling: %+v\n", err)
      } else {
        timestamp := binary.LittleEndian.Uint32(k)
        fmt.Printf("%d -> %s\n\n", timestamp, pkt.String())
      }
    }
    return nil
  })
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
    //nni := tx.Bucket([]byte("nodeindex"))
    c := n.Cursor()
    for k, v := c.First(); k != nil; k, v = c.Next() {
      nodeInfo := pb.NodeInfo{}
      if err := proto.Unmarshal(v, &nodeInfo); err != nil {
        fmt.Printf("Error unmarshalling: %+v\n", err)
        log.Fatal("not good")
      }

      nodeNumber := fmt.Sprintf("!%08x", nodeInfo.Num)
      if nodeInfo.User.Id != nodeNumber {
        fmt.Printf("%s %s\n", nodeInfo.User.Id, nodeNumber)
      }
      
      //bs := make([]byte, 4)
      //binary.LittleEndian.PutUint32(bs, nodeInfo.Num)
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

func SaveNodeToDb(nodeInfo *pb.NodeInfo) error {
  out, err := proto.Marshal(nodeInfo)
  if err != nil {			
    return err
  }

  //fmt.Printf("%+v\n", out)
  fmt.Printf("Saving node %s\n", nodeInfo.User.LongName)
  return db.Update(func(tx *bolt.Tx) error {
    // update the node
    b := tx.Bucket([]byte("nodes"))
    b.Put([]byte(nodeInfo.User.Id), out)

    // update the index too
    bni := tx.Bucket([]byte("nodeindex"))
    nodeNumBytes := make([]byte, 4)
    binary.LittleEndian.PutUint32(nodeNumBytes, nodeInfo.Num)
    bni.Put(nodeNumBytes, []byte(nodeInfo.User.Id))
    return nil
  })
}

func RemoveNodeFromDb(nodeId string) {
  fmt.Printf("Removing node %s\n", nodeId)
  db.Update(func(tx *bolt.Tx) error {
    // update the node
    b := tx.Bucket([]byte("nodes"))
    b.Delete([]byte("!" + nodeId))

    return nil
  })
}

func AppendMessagePktToDb(timestamp uint32, pktData []byte) {
  db.Update(func(tx *bolt.Tx) error {

    // update the node
    b := tx.Bucket([]byte("messages"))
    timestampBytes := make([]byte, 4)
    binary.LittleEndian.PutUint32(timestampBytes, timestamp)
    b.Put(timestampBytes, pktData)

    return nil
  })
}

func IterateMessagesFromDb(cb func(n []byte)) {
  db.View(func(tx *bolt.Tx) error {

    b := tx.Bucket([]byte("messages"))
    c := b.Cursor()
    for k, v := c.First(); k != nil; k, v = c.Next() {
      cb(v)
    }

    return nil
  })
  db.Update(func(tx *bolt.Tx) error {
    b := tx.Bucket([]byte("messages"))
    c := b.Cursor()
    for k, _ := c.First(); k != nil; k, _ = c.Next() {
      b.Delete(k)
    }
    return nil
  })
}

func ClearUnknownNodesFromDb() {
	db.Update(func(tx *bolt.Tx) error {

		counter := 0
    b := tx.Bucket([]byte("nodes"))
    c := b.Cursor()
    for k, v := c.First(); k != nil; k, v = c.Next() {			
      nodeInfo := pb.NodeInfo{}
      if err := proto.Unmarshal(v, &nodeInfo); err != nil {
        log.Fatal(err)
      }

			if nodeInfo.User.Macaddr == nil {
				counter++
				//fmt.Printf("%+v\n", nodeInfo)
				b.Delete(k)
			}
		}

		fmt.Printf("removed %d unknown nodes\n", counter)

		return nil
	})
}