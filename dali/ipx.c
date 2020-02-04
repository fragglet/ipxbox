
#include <dos.h>
#include <i86.h>
#include <stdlib.h>

#include "inlines.h"

#define IPX_INTERRUPT 0x7a
#define REDIRECTOR_INTERRUPT 0x2f

#define MAX_OPEN_SOCKETS 8

#define IPX_CMD_OPEN_SOCKET   0x0000
#define IPX_CMD_CLOSE_SOCKET  0x0001
#define IPX_CMD_GET_LOCAL_TGT 0x0002
#define IPX_CMD_SEND_PACKET   0x0003
#define IPX_CMD_LISTEN_PACKET 0x0004
#define IPX_CMD_SCHED_EVENT   0x0005
#define IPX_CMD_CANCEL_OP     0x0006
#define IPX_CMD_SCHED_SPEC    0x0007
#define IPX_CMD_GET_INTERVAL  0x0008
#define IPX_CMD_GET_ADDRESS   0x0009
#define IPX_CMD_RELINQUISH    0x000a
#define IPX_CMD_DISCONNECT    0x000b
#define IPX_CMD_GET_PKT_SIZE  0x000d
#define IPX_CMD_SPX_INSTALLED 0x0010
#define IPX_CMD_GET_MTU       0x001a

#define MTU 576

struct ipx_socket {
	unsigned short socket;
};

static void (__interrupt far *old_isr)(void);
static void (__interrupt far *next_redirector)(void);
static struct ipx_socket open_sockets[MAX_OPEN_SOCKETS];
static unsigned int num_open_sockets;

static struct ipx_socket *FindSocket(unsigned short num)
{
	int i;

	for (i = 0; i < MAX_OPEN_SOCKETS; ++i) {
		if (open_sockets[i].socket == num) {
			return &open_sockets[i];
		}
	}

	return NULL;
}

static unsigned short OpenSocket(unsigned int num)
{
	struct ipx_socket *sock;

	if (num == 0) {
		num = 0x4002;
		while (FindSocket(num) != NULL) {
			++num;
		}
	}

	// Already in use?
	if (FindSocket(num) != NULL) {
		return 0xff;
	}

	sock = FindSocket(0);
	if (sock == NULL) {
		return 0xfe;
	}

	sock->socket = num;
	// TODO: DX for socket number
	return 0;
}

static void CloseSocket(unsigned int num)
{
	struct ipx_socket *sock;

	if (num == 0) {
		return;
	}

	sock = FindSocket(num);
	if (sock == NULL) {
		return;
	}

	sock->socket = 0;
}

static void __interrupt IPX_ISR(union INTPACK ip)
{
	switch (ip.w.bx) {
		case IPX_CMD_OPEN_SOCKET:
			ip.w.ax = OpenSocket(htons(ip.w.dx));
			break;
		case IPX_CMD_CLOSE_SOCKET:
			CloseSocket(htons(ip.w.dx));
			break;
		case IPX_CMD_GET_LOCAL_TGT:
			// TODO
			break;
		case IPX_CMD_SEND_PACKET:
			// TODO
			break;
		case IPX_CMD_LISTEN_PACKET:
			// TODO
			break;
		case IPX_CMD_SCHED_EVENT:
			// TODO
			break;
		case IPX_CMD_CANCEL_OP:
			// TODO
			break;
		case IPX_CMD_SCHED_SPEC:
			// TODO
			break;
		case IPX_CMD_GET_INTERVAL:
			// TODO
			break;
		case IPX_CMD_GET_ADDRESS:
			// TODO
			break;
		case IPX_CMD_RELINQUISH:
		case IPX_CMD_DISCONNECT:
			// no-op
			break;
		case IPX_CMD_GET_PKT_SIZE:
			ip.w.ax = 1024;
			ip.w.cx = 0;
			break;
		case IPX_CMD_SPX_INSTALLED:
			ip.w.ax = 0;
			break;
		case IPX_CMD_GET_MTU:
			ip.w.ax = MTU;
			ip.w.cx = 0;
			break;
		default:
			break;
	}
}

static void __interrupt RedirectorISR(union INTPACK ip)
{
	if (ip.w.ax == 0x7a00) {
		ip.h.al = 0xff;
		return;
	}

	// TODO: Answer IPX API requests on this ISR too?

	_chain_intr(next_redirector);
}

static void UnhookVector(void)
{
	_disable();
	_dos_setvect(IPX_INTERRUPT, old_isr);
	_dos_setvect(REDIRECTOR_INTERRUPT, next_redirector);
	_enable();
}

void HookIPXVector(void)
{
	_disable();
	old_isr = _dos_getvect(IPX_INTERRUPT);
	_dos_setvect(IPX_INTERRUPT, IPX_ISR);
	next_redirector = _dos_getvect(REDIRECTOR_INTERRUPT);
	_dos_setvect(REDIRECTOR_INTERRUPT, RedirectorISR);
	_enable();

	atexit(UnhookVector);
}

