# Maszyny wirtualne

[English](../virtual-machines.md)

Obraz x86-64 (`suplaos-<wersja>-amd64.img.xz`) działa też w maszynach
wirtualnych. Każdy hiperwizor potrzebuje:

- firmware UEFI, nie starego BIOS-u, i wyłączonego Secure Boot (program
  startowy nie jest podpisany),
- co najmniej 2 GB pamięci, 2 procesorów i dysku 16 GB,
- sieci mostkowanej, żeby maszyna dostała adres z routera, a `suplaos.local`
  działało jak z prawdziwym urządzeniem.

Najpierw rozpakuj obraz (`xz -d suplaos-<wersja>-amd64.img.xz`, na Windowsie
zrobi to 7-Zip). Dysk można powiększyć przed pierwszym startem, SuplaOS sam
zajmie całe miejsce.

## Proxmox VE

1. Utwórz maszynę: OS „Do not use any media”, System: Machine `q35`, BIOS
   `OVMF (UEFI)`, dodaj dysk EFI bez „Pre-Enroll keys” (wtedy Secure Boot
   jest wyłączony), zaznacz „Qemu Agent”. Disks: usuń domyślny dysk.
   Network: most `vmbr0`.
2. Skopiuj rozpakowany obraz na serwer Proxmox i zaimportuj go, np. dla
   maszyny 100 i magazynu `local-lvm`:

       qm importdisk 100 suplaos-<wersja>-amd64.img local-lvm
       qm set 100 --scsihw virtio-scsi-pci --scsi0 local-lvm:vm-100-disk-1
       qm resize 100 scsi0 16G
       qm set 100 --boot order=scsi0

   Nazwę zaimportowanego dysku sprawdź w wyniku `qm importdisk`.
3. Uruchom maszynę. Jej adres widać w konsoli, a dzięki agentowi także
   w podsumowaniu (Summary) maszyny.

## VirtualBox

1. Przekonwertuj obraz i powiększ dysk:

       VBoxManage convertfromraw suplaos-<wersja>-amd64.img suplaos.vdi --format VDI
       VBoxManage modifymedium suplaos.vdi --resize 16384

2. Utwórz maszynę (Linux, Debian 64-bit) z istniejącym dyskiem `suplaos.vdi`,
   w System włącz „Enable EFI”, w Network wybierz „Bridged Adapter”.

## VMware Workstation / Player

1. Przekonwertuj obraz (qemu-img jest w pakiecie `qemu-utils` na Linuksie
   i w QEMU dla Windowsa):

       qemu-img convert -O vmdk suplaos-<wersja>-amd64.img suplaos.vmdk

2. Utwórz maszynę „Debian 12 64-bit” z tym dyskiem, ustaw firmware UEFI bez
   Secure Boot (Options, Advanced) i sieć mostkowaną.

## Hyper-V

1. Przekonwertuj obraz do VHDX:

       qemu-img convert -O vhdx -o subformat=dynamic suplaos-<wersja>-amd64.img suplaos.vhdx

2. Utwórz maszynę generacji 2 z tym dyskiem, wyłącz Secure Boot (Ustawienia,
   Zabezpieczenia) i podłącz ją do zewnętrznego przełącznika wirtualnego.

Z WSL zrobi to wszystko `tools/hyperv.sh` z tego repozytorium.
