{-# LANGUAGE OverloadedStrings #-}
import Call
import Control.Concurrent (threadDelay)
import Control.Exception (try, SomeException)
import qualified Data.Aeson.KeyMap as KM
import Data.Aeson (Value (Number))
import System.Directory (makeAbsolute)
import System.IO.Temp (withSystemTempDirectory)
import System.FilePath ((</>))
import System.Posix.Signals (signalProcess, nullSignal)
import Test.Tasty (defaultMain, testGroup)
import Test.Tasty.HUnit
import GHC.Clock (getMonotonicTime)

main :: IO ()
main = do
  stub <- makeAbsolute "../claude"
  defaultMain $ testGroup "call"
    [ testCase "ok" $ do
        o <- call stub [("STUB_MODE", "ok")] 10000000
        case o of
          Ok m -> KM.lookup "n" m @?= Just (Number 3)
          _ -> assertFailure (show o)
    , testCase "null" $ call stub [("STUB_MODE", "nullout")] 10000000 >>= (@?= Null)
    , testCase "hang killed" $ withSystemTempDirectory "hs" $ \d -> do
        let f = d </> "pid"
        t0 <- getMonotonicTime
        o <- call stub [("STUB_MODE", "hang"), ("STUB_PIDFILE", f)] 1000000
        t1 <- getMonotonicTime
        o @?= Timeout
        assertBool "fast" (t1 - t0 < 3)
        threadDelay 100000
        pid <- read <$> readFile f
        r <- try (signalProcess nullSignal pid) :: IO (Either SomeException ())
        assertBool "grandchild dead" (either (const True) (const False) r)
    , testCase "parallel" $ parallelCalls stub 8 >>= assertBool "all ok" . all isOk
    ]
  where isOk (Ok _) = True
        isOk _ = False
