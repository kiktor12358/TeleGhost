/*
 * C wrapper for i2pd C++ API
 * This file provides C-compatible functions that can be called via CGO
 *
 * Build with:
 *   g++ -std=c++17 -c i2pd_wrapper.cpp -I../i2pd/libi2pd -I../i2pd/i18n
 * -I../i2pd
 */

#include <atomic>
#include <cstring>
#include <fstream>
#include <memory>
#include <string>
#include <thread>

// i2pd includes
#include "ClientContext.h"
#include "Config.h"
#include "FS.h"
#include "Log.h"
#include "RouterContext.h"
#include "api.h"

static std::atomic<bool> g_running{false};
static std::string g_b32_address;
static std::string g_datadir;

extern "C" {

// Initialize i2pd with configuration
void i2pd_init(const char *datadir, int sam_enabled, int sam_port) {
  g_datadir = datadir ? datadir : ".i2pd";

  // Prepare arguments for i2pd
  std::vector<const char *> args;
  args.push_back("teleghost"); // argv[0]

  std::string datadir_arg = "--datadir=" + g_datadir;
  args.push_back(datadir_arg.c_str());

  // SAM configuration
  if (sam_enabled) {
    args.push_back("--sam.enabled=true");
    std::string sam_port_arg = "--sam.port=" + std::to_string(sam_port);
    args.push_back(sam_port_arg.c_str());
  } else {
    args.push_back("--sam.enabled=false");
  }

  // Enable NTCP2 and SSU2 for NAT traversal
  args.push_back("--ntcp2.enabled=true");
  args.push_back("--ssu2.enabled=true");

  // Disable HTTP console and other services we don't need
  args.push_back("--http.enabled=false");
  args.push_back("--httpproxy.enabled=false");
  args.push_back("--socksproxy.enabled=false");
  args.push_back("--bob.enabled=false");
  args.push_back("--i2cp.enabled=false");
  args.push_back("--i2pcontrol.enabled=false");

  // Floodfill off for client mode
  args.push_back("--floodfill=false");

  // Logging
  args.push_back("--log=file");
  std::string log_arg = "--logfile=" + g_datadir + "/i2pd.log";
  args.push_back(log_arg.c_str());
  args.push_back("--loglevel=warn");

  // Bandwidth settings for faster bootstrap
  args.push_back("--bandwidth=O"); // 256 Kbps

  // Initialize i2pd
  i2p::api::InitI2P(args.size(), const_cast<char **>(args.data()), "TeleGhost");
}

// Start i2pd router
void i2pd_start() {
  if (g_running)
    return;

  // Start with no log stream (use file logging)
  i2p::api::StartI2P(nullptr);
  g_running = true;
}

// Stop i2pd router
void i2pd_stop() {
  if (!g_running)
    return;

  i2p::api::StopI2P();
  g_running = false;
}

// Terminate and cleanup
void i2pd_terminate() {
  i2p::api::TerminateI2P();
  g_b32_address.clear();
}

// Check if router is running
int i2pd_is_running() { return g_running ? 1 : 0; }

// Get router's B32 address
const char *i2pd_get_b32_address() {
  if (!g_running)
    return nullptr;

  try {
    auto &context = i2p::context;
    auto ident = context.GetRouterInfo().GetIdentHash();
    g_b32_address = ident.ToBase32() + ".b32.i2p";
    return g_b32_address.c_str();
  } catch (...) {
    return nullptr;
  }
}

} // extern "C"
