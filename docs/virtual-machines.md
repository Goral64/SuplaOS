# Virtual machines

[Po polsku](pl/virtual-machines.md)

The x86-64 image (`suplaos-<version>-amd64.img.xz`) runs in virtual machines
as well. What every hypervisor needs:

- UEFI firmware, not the old BIOS, and Secure Boot turned off (the boot
  loader isn't signed),
- at least 2 GB of memory, 2 CPUs and a 16 GB disk,
- a bridged network, so that the VM gets an address from your router and
  `suplaos.local` works like with a real device.

Unpack the image first (`xz -d suplaos-<version>-amd64.img.xz`, on Windows
7-Zip does it). The disk can be made bigger before the first start, SuplaOS
takes all the space on its own.

## Proxmox VE

1. Create a VM: OS "Do not use any media", System: Machine `q35`, BIOS
   `OVMF (UEFI)`, add an EFI disk without "Pre-Enroll keys" (that keeps
   Secure Boot off), tick "Qemu Agent". Disks: remove the default disk.
   Network: bridge `vmbr0`.
2. Copy the unpacked image to the Proxmox host and import it, e.g. for VM
   100 and the `local-lvm` storage:

       qm importdisk 100 suplaos-<version>-amd64.img local-lvm
       qm set 100 --scsihw virtio-scsi-pci --scsi0 local-lvm:vm-100-disk-1
       qm resize 100 scsi0 16G
       qm set 100 --boot order=scsi0

   Check the name of the imported disk in the output of `qm importdisk`.
3. Start the VM. Its address is shown in the console and, thanks to the
   agent, in the Summary of the VM.

## VirtualBox

1. Convert the image and make the disk bigger:

       VBoxManage convertfromraw suplaos-<version>-amd64.img suplaos.vdi --format VDI
       VBoxManage modifymedium suplaos.vdi --resize 16384

2. Create a VM (Linux, Debian 64-bit), use the existing disk `suplaos.vdi`,
   in System turn on "Enable EFI", in Network choose "Bridged Adapter".

## VMware Workstation / Player

1. Convert the image (qemu-img is in the `qemu-utils` package on Linux and
   in QEMU for Windows):

       qemu-img convert -O vmdk suplaos-<version>-amd64.img suplaos.vmdk

2. Create a VM for "Debian 12 64-bit" with this disk, set the firmware to
   UEFI without Secure Boot (Options, Advanced) and the network to bridged.

## Hyper-V

1. Convert the image to VHDX:

       qemu-img convert -O vhdx -o subformat=dynamic suplaos-<version>-amd64.img suplaos.vhdx

2. Create a Generation 2 VM with this disk, turn off Secure Boot (Settings,
   Security) and connect it to an external virtual switch.

From WSL `tools/hyperv.sh` in this repository does all of it.
