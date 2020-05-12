netdeck
-------
netdeck is a simple command-line tool for managing the hidden information that is inherently a part of many card- or boardgames in which each player has a "hand" of cards that are visible only to them.  netdeck makes it easy to play most games of this nature with your friends over the internet, without the need for it to have pre-existing support for the particular game you would like to play.

### Getting started
Firstly, you'll need to download the latest version for your operating system from [the releases page](https://github.com/jacquesh/netdeck/releases). Extract the executable from the archive you downloaded and run it. This will connect you to the default server (assuming it is running at the time) and prompt you for a name/alias to use.

If you'd like to run your own server, specify your name ahead of time, or otherwise customise the functionality of netdeck, you'll need to run it from the command line with some arguments. `netdeck -m server` will run a server locally on your computer that you can connect to using `netdeck -s localhost`. For more information, run `netdeck --help`.

### How does it work?
A netdeck game comes in two parts: Some clients (one run by each player) and a server (which handles many or all clients across games). To begin, one player must write a "game specification" file, which is just a human-readable text file in a specific format ([the game has the specification for a standard 52-card deck built-in](https://github.com/jacquesh/netdeck/blob/master/gamespec.go#L115)).  That player must then instruct the server to create a new game instance using their spec file.  The server will setup the game and tell the creating player how other players can join their game.

Once in the game, netdeck provides a set of generic commands to each player, which allow them to manipulate the cards in their hand (for example by drawing, discarding, showing cards to other players, etc). At this point it is up to the players what they would like to do - in the same way that there is nothing stopping you from drawing a card from a shared deck at any point while sitting around a table (regardless of whether the game's rules instruct or allow you to do so), so netdeck will not enforce any behaviour by the players. However just as when sitting around a table, netdeck will make sure that all other players know of any relevant actions you take, so no uncalled-for peeking at the cards on top of the deck!

In the same way the your regular boardgame does not itself facilitate communication in any way (you do that just by speaking!), so netdeck does not aid in communication between players in any useful way outside of allowing players to show cards to one another. The suggested setup is that all players in a game are also in a group video call (using [Whereby](https://whereby.com/), [Jitsi Meet](https://meet.jit.si/), Microsoft Teams, or Google Hangouts for example).

