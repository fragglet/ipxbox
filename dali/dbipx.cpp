
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
#define MTU 576

static uint8_t buf[MTU];
static IpAddr_t server_addr;
static int udp_port;
static int registered;
static struct ipx_address local_addr;

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

	if (ntohs(udp->len) < sizeof(struct ipx_header)) {
		return;
	}
	ipx = (const struct ipx_header *) packet;
	if (ntohs(ipx->src.socket) == 2 && ntohs(ipx->dest.socket) == 2) {
		registered = 1;
		memcpy(&local_addr, &ipx->dest, sizeof(struct ipx_address));
		return;
	}
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
		PACKET_PROCESS_SINGLE;
		Arp::driveArp( );
	}
}

static void ResolveAddress(const char *addr)
{
	if (Dns::resolve(addr, server_addr, 1) < 0) {
		Error("Error resolving server address '%s'", addr);
	}

	while (Dns::isQueryPending()) {
		PACKET_PROCESS_SINGLE;
		Arp::driveArp();
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
		Error("No response from server at %s:%d", addr, port);
	}
}

}

