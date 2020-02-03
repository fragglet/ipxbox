MTCP_DIR = \mtcp
TCP_H_DIR = $(MTCP_DIR)\TCPINC
TCP_C_DIR = $(MTCP_DIR)\TCPLIB
COMMON_H_DIR = $(MTCP_DIR)\INCLUDE

MEMORY_MODEL = -ms
CFLAGS = -0 $(MEMORY_MODEL) -DCFG_H="mtcpcomp.cfg" -oh -ok -os -s -oa -ei -zp2 -zpw -we
CFLAGS += -i=$(TCP_H_DIR) -i=$(COMMON_H_DIR)

MTCP_OBJS = packet.obj dns.obj arp.obj eth.obj ip.obj udp.obj utils.obj timer.obj ipasm.obj trace.obj
OBJS = dbipx.obj dali.obj

all : dali.exe

clean : .symbolic
	@del dali.exe
	@del *.obj
	@del *.map

.asm : $(TCP_C_DIR)

.cpp : $(TCP_C_DIR)

.asm.obj :
	wasm -0 $(MEMORY_MODEL) $[*

.cpp.obj :
	wpp $[* $(CFLAGS)

.c.obj
	wcc $[* $(CFLAGWS)

dali.exe : $(MTCP_OBJS) $(OBJS)
	wlink system dos option map option eliminate option stack=4096 name $@ file *.obj

