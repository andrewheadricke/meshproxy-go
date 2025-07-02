# meshproxy-go

A cli tool to connect to serial meshtastic nodes and provide a TCP server for other clients to connect to. It defaults to listen on port 4403 just like on the hardware devices with Wifi.

### Why?
Because I have an externally mounted RAK Wisblock device paired with a Raspberry pi zero, the RAK only has Serial and Bluetooth and I wanted to share its packet stream with other devices on the network using the Raspberry pi zero's Wifi.

#### Features
* Allows multiple clients to connect via TCP simultanously to the single node. All packets received on the node will be forwarded to all TCP connections. All TCP clients can send packets to the node.

* Auto switch serial ports. The serial connection appears unstable and disconnects at random times (could be my USB plug), and occasionaly would change from ttyUSB0 to ttyUSB1. This tool will auto switch between 0 & 1.

* Will store all the discovered nodes in a separate database allowing more than the 80-100 max nodes supported by the meshtastic hardware. When TCP clients connect they will be sent all the nodes in the database instead of the 80-100 nodes on the device. This is particularly important if your mesh is nearing the 80-100 limit as you will start to get constant "new node" notifcations as nodes drop off and are re-added.

* Will store all unread messages if no TCP clients are connected. Only the first TCP connection will receive the stored messages.

* Protobuf format up to date as of firmware v2.6.11

* Much faster node sync for app clients

* In the event your meshtastic device gets corrupted or resets you have a backup of your node list

### Notes
There is no security or authorization, it's a straight through proxy. Anyone that can connect to the TCP port will receive all settings on the meshtastic device. Things like Private/Public keys for the node and channels, Wifi settings incl password, MQTT username and password etc.

On some devices like RAK, establishing a serial connection appears to disable the bluetooth. So if you use this tool you may not be able to connect via bluetooth. Power cycle the node to allow bluetooth again.