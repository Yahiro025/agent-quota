#include "protocol.h"
#include <cstdio>
#include <vector>

int main() {
 int failures=0;
 auto check=[&](bool value,const char* name){printf("%s %s\n",value?"PASS":"FAIL",name);if(!value)++failures;};
 PacketHeader h{PacketMagic,PacketConnect|PacketVisible,123,2,2,120,ProtocolVersion};
 std::vector<unsigned char> bytes(sizeof(h)+16,0xff);
 memcpy(bytes.data(),&h,sizeof(h));
 check(DecodePacket(bytes.data(),bytes.size()).valid,"valid owned pixel frame");
 check(!DecodePacket(bytes.data(),bytes.size()-1).valid,"reject truncated pixels");
 check(!DecodePacket(bytes.data(),bytes.size()+1).valid,"reject trailing bytes");
 check(!DecodePacket(nullptr,0).valid,"reject absent payload");
 auto bad=h;bad.width=0xffffffff;memcpy(bytes.data(),&bad,sizeof(bad));
 check(!DecodePacket(bytes.data(),bytes.size()).valid,"reject dimension overflow");
 bad=h;bad.flags=128;memcpy(bytes.data(),&bad,sizeof(bad));
 check(!DecodePacket(bytes.data(),bytes.size()).valid,"reject unknown operation");
 bad=h;bad.version=99;memcpy(bytes.data(),&bad,sizeof(bad));
 check(!DecodePacket(bytes.data(),bytes.size()).valid,"reject incompatible protocol");
 bad=h;bad.generation=0;memcpy(bytes.data(),&bad,sizeof(bad));
 check(!DecodePacket(bytes.data(),bytes.size()).valid,"reject absent connection generation");
 h.flags=PacketDisconnect;h.width=h.height=0;bytes.resize(sizeof(h));memcpy(bytes.data(),&h,sizeof(h));
 check(DecodePacket(bytes.data(),bytes.size()).valid,"explicit zero-pixel disconnect");
 printf("failures=%d\n",failures);
 return failures?1:0;
}
