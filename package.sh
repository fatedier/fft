# compile for version
make
if [ $? -ne 0 ]; then
    echo "make error"
    exit 1
fi

fft_version=`./bin/fft --version`
echo "build version: $fft_version"

# cross_compiles
make -f ./Makefile.cross-compiles

rm -rf ./packages
mkdir ./packages

os_all='linux windows darwin freebsd'
arch_all='386 amd64 arm arm64 mips64 mips64le mips mipsle'

for os in $os_all; do
    for arch in $arch_all; do
        fft_dir_name="fft_${fft_version}_${os}_${arch}"
        fft_path="./packages/fft_${fft_version}_${os}_${arch}"

        if [ "x${os}" = x"windows" ]; then
            if [ ! -f "./fft_${os}_${arch}.exe" ]; then
                continue
            fi
            if [ ! -f "./fftw_${os}_${arch}.exe" ]; then
                continue
            fi
            if [ ! -f "./ffts_${os}_${arch}.exe" ]; then
                continue
            fi
            mkdir ${fft_path}
            mv ./fft_${os}_${arch}.exe ${fft_path}/fft.exe
            mv ./fftw_${os}_${arch}.exe ${fft_path}/fftw.exe
            mv ./ffts_${os}_${arch}.exe ${fft_path}/ffts.exe
        else
            if [ ! -f "./fft_${os}_${arch}" ]; then
                continue
            fi
            if [ ! -f "./fftw_${os}_${arch}" ]; then
                continue
            fi
            if [ ! -f "./ffts_${os}_${arch}" ]; then
                continue
            fi
            mkdir ${fft_path}
            mv ./fft_${os}_${arch} ${fft_path}/fft
            mv ./fftw_${os}_${arch} ${fft_path}/fftw
            mv ./ffts_${os}_${arch} ${fft_path}/ffts
        fi  
        cp ./LICENSE ${fft_path}

        # packages
        cd ./packages
        if [ "x${os}" = x"windows" ]; then
            zip -rq ${fft_dir_name}.zip ${fft_dir_name}
        else
            tar -zcf ${fft_dir_name}.tar.gz ${fft_dir_name}
        fi  
        cd ..
        rm -rf ${fft_path}
    done
done
