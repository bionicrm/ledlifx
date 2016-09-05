# LED-LIFX
Raspberry Pi LED controller for LIFX.

## Custom Usage
The code is rather personalized, and not much thought was put into different scenarios. Therefore, should you wish to use this for yourself, you should fork it and make the following changes to suite your needs:
- run.sh
  - The GPIO pins may be different for you, so you might want to modify the message as needed
  - The SSH user, IP, and `rsync` destination may be different
- main.go
  - `main()` will probably be modified to fit your specific GPIO pins, platform, and the like. See [gobot.io](https://gobot.io) for more on preparing the controller
  - If you run the controller on more than one machine, try modifying the last few bytes of the mocked MAC address in `Start()` for each one so that all machines have a unique MAC
  - Modify the constants in `configureBulb(...)` to fit your needs, such as the label, location label, and group label. Also, should a LIFX firmware update occur, you may want to update the host firmware build/version and the wifi firmware build. You'll want to copy a few values from an existing LIFX bulb. To obtain them, use the [Clifx](https://github.com/lifx-tools/clifx) tool:
    - bulb.hostFimware.{build,version} = `clifx -c 1 -p hostfirmware`
    - bulb.wifiFirwmare.build = `clifx -c 1 -p wififirmware`
    - bulb.location.{location,label,updatedAt} = `clifx -c 1 -p location`
    - bulb.group.{group,label,updatedAt} = `clifx -c 1 -p group`
    - bulb.owner.{owner,updatedAt} = `clifx -c 1 -p owner`
