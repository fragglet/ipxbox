
// Implementation of DOSbox UDP protocol, using the DOS mTCP stack.

#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

extern "C" {
#include "ipx.h"
}

#include "dbipx.h"

#include "arp.h"
#include "dns.h"
#include "packet.h"
#include "timer.h"
#include "udp.h"

#define REG_ATTEMPTS 5

static IpAddr_t server_addr;
static int udp_port;
static int registered;
static dbipx_packet_callback rx_callback = NULL;
struct ipx_address dbipx_local_addr;

extern "C" {

// Aborts the program with an abnormal program termination.
void Error(char *fmt, ...)
{
	va_list args;

	va_start(args, fmt);
	vfprintf(stderr, fmt, args);
	va_end(args);

	exit(1);
}

static void PacketReceived(const unsigned char *packet, const UdpHeader *udp)
{
	const struct ipx_header *ipx;
	unsigned int len = ntohs(udp->len);

	if (len < sizeof(struct ipx_header)) {
		Buffer_free(packet);
		return;
	}

	ipx = (const struct ipx_header *) (packet + sizeof(UdpPacket_t));
	if (ntohs(ipx->src.socket) == 2 && ntohs(ipx->dest.socket) == 2) {
		registered = 1;
		memcpy(&dbipx_local_addr, &ipx->dest, sizeof(struct ipx_address));
	} else if (rx_callback != NULL) {
		rx_callback(ipx, len);
	}

	Buffer_free(packet);
}

static void SendRegistration(void)
{
	static struct ipx_header tmphdr;

	memset(&tmphdr, 0, sizeof(tmphdr));
	tmphdr.dest.socket = htons(2);
	tmphdr.src.socket = htons(2);
	tmphdr.checksum = htons(0xffff);
	tmphdr.length = htons(0x1e);
	tmphdr.transport_control = 0;
	tmphdr.type = 0xff;

	Udp::sendUdp(server_addr, udp_port, udp_port,
	             sizeof(tmphdr), (unsigned char *) &tmphdr, 0);
}

static void Delay(int timer_ticks)
{
	clockTicks_t start = TIMER_GET_CURRENT();

	while (Timer_diff(start, TIMER_GET_CURRENT()) < timer_ticks) {
		DBIPX_Poll();
	}
}

static void ResolveAddress(const char *addr)
{
	if (Dns::resolve(addr, server_addr, 1) < 0) {
		Error("Error resolving server address '%s'", addr);
	}

	while (Dns::isQueryPending()) {
		DBIPX_Poll();
		Dns::drivePendingQuery();
	}

	if (Dns::resolve(addr, server_addr, 0) != 0) {
		Error("Failed to resolve server address '%s'", addr);
	}
}

static void __interrupt __far CtrlBreakHandler() {
}

static void ShutdownStack(void)
{
	Utils::endStack();
	Utils::dumpStats(stdout);
}

void DBIPX_Connect(const char *addr, int port)
{
	int i;

	if (Utils::parseEnv() != 0) {
		Error("Error parsing environment for mTCP initialization.");
	}

	if (Utils::initStack(0, 0, CtrlBreakHandler, CtrlBreakHandler)) {
		Error("Error initializing TCP/IP stack.");
	}
	atexit(ShutdownStack);

	ResolveAddress(addr);
	udp_port = port;

	registered = 0;
	if (Udp::registerCallback(port, PacketReceived) != 0) {
		Error("Failed to register UDP callback function");
	}

	Delay(TIMER_TICKS_PER_SEC);

	for (i = 0; !registered && i < REG_ATTEMPTS*TIMER_TICKS_PER_SEC; ++i) {
		if ((i % TIMER_TICKS_PER_SEC) == 0) {
			SendRegistration();
		}
		Delay(1);
	}

	if (!registered) {
		Error("No response from server at %d.%d.%d.%d:%d",
		      server_addr[0], server_addr[1], server_addr[2],
		      server_addr[3], port);
	}
}

void DBIPX_SendPacket(struct ipx_header *pkt, size_t len)
{
	Udp::sendUdp(server_addr, udp_port, udp_port,
	             len, (uint8_t *) pkt, 0);
}

void DBIPX_SetCallback(dbipx_packet_callback callback)
{
	rx_callback = callback;
}

void DBIPX_Poll(void)
{
	while (Buffer_first != Buffer_next) {
		PACKET_PROCESS_SINGLE;
		Arp::driveArp();
	}
}

}

