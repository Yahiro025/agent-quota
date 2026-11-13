#pragma once
#include <cstdint>
#include <cstddef>
#include <cstring>

inline constexpr uint32_t PacketMagic=0x31425141;
inline constexpr uint32_t ProtocolVersion=2;
inline constexpr uint32_t PacketVisible=1,PacketDisconnect=2,PacketConnect=4;
inline constexpr uint32_t MaxWidth=1024,MaxHeight=256;
struct PacketHeader {
 uint32_t magic,flags;
 uint64_t generation;
 uint32_t width,height,dpi,version;
};
static_assert(sizeof(PacketHeader)==32);
struct Packet {bool valid=false;PacketHeader header{};const unsigned char* pixels=nullptr;};
inline Packet DecodePacket(const void* data,size_t length) {
 if(!data || length<sizeof(PacketHeader))return {};
 PacketHeader h{};memcpy(&h,data,sizeof(h));
 if(h.magic!=PacketMagic || h.version!=ProtocolVersion || !h.generation || (h.flags&~7u) || h.dpi<96 || h.dpi>768)return {};
 if(h.flags&PacketDisconnect) {
  if(h.flags!=PacketDisconnect || h.width || h.height || length!=sizeof(h))return {};
  return {true,h,nullptr};
 }
 if(!h.width || !h.height || h.width>MaxWidth || h.height>MaxHeight || uint64_t(h.width)*h.height*4+sizeof(h)!=length)return {};
 return {true,h,static_cast<const unsigned char*>(data)+sizeof(h)};
}
